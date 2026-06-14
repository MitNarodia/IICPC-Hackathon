package leaderboard

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/iicpc/track3/telemetry-engine/pkg/models"
	"github.com/iicpc/track3/telemetry-engine/pkg/store"
	logpkg "github.com/iicpc/track3/telemetry-engine/pkg/telemetry"
)

// Filter narrows a board query. Zero values mean "no constraint".
type Filter struct {
	MinCorrectness float64 // keep entries with correctness_score*100 >= this
	Health         string  // exact health match (READY/DEGRADED/UNHEALTHY)
	Search         string  // case-insensitive substring of display name / submission id
	Limit          int     // top-N (0 = all)
}

func (f Filter) keep(e *models.LeaderboardEntry) bool {
	if f.MinCorrectness > 0 && e.CorrectnessScore < f.MinCorrectness {
		return false
	}
	if f.Health != "" && !strings.EqualFold(e.Health, f.Health) {
		return false
	}
	if f.Search != "" {
		s := strings.ToLower(f.Search)
		if !strings.Contains(strings.ToLower(e.DisplayName), s) &&
			!strings.Contains(strings.ToLower(e.SubmissionID), s) {
			return false
		}
	}
	return true
}

// API serves the leaderboard over REST and upgrades WebSocket subscriptions.
type API struct {
	svc      *Service
	hub      *Hub
	pg       *store.Postgres
	log      *logpkg.Logger
	upgrader websocket.Upgrader
}

// NewAPI wires the HTTP surface.
func NewAPI(svc *Service, hub *Hub, pg *store.Postgres, log *logpkg.Logger) *API {
	return &API{
		svc: svc, hub: hub, pg: pg, log: log,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1 << 10,
			WriteBufferSize: 1 << 14,
			CheckOrigin:     func(*http.Request) bool { return true },
		},
	}
}

// Routes registers the API endpoints.
func (a *API) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/runs", a.withCORS(a.handleRuns))
	mux.HandleFunc("/v1/leaderboard", a.withCORS(a.handleLeaderboard))
	mux.HandleFunc("/v1/contestant", a.withCORS(a.handleContestant))
	mux.HandleFunc("/v1/ws/leaderboard", a.handleWS)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
}

// handleRuns lists active runs for the selector.
func (a *API) handleRuns(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"runs": a.svc.Runs()})
}

// handleLeaderboard returns the ranked board for ?run=, with optional filters.
func (a *API) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	run := r.URL.Query().Get("run")
	if run == "" {
		http.Error(w, "missing ?run", http.StatusBadRequest)
		return
	}
	f := Filter{
		Health: r.URL.Query().Get("health"),
		Search: r.URL.Query().Get("search"),
	}
	if v := r.URL.Query().Get("min_correctness"); v != "" {
		f.MinCorrectness, _ = strconv.ParseFloat(v, 64)
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		f.Limit, _ = strconv.Atoi(v)
	}
	entries := a.svc.Snapshot(run, f)
	if f.Limit > 0 && len(entries) > f.Limit {
		entries = entries[:f.Limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":  run,
		"entries": entries,
		"count":   len(entries),
		"served":  time.Now().UTC(),
	})
}

// ContestantDetail is the deep-dive payload for one submission: its live entry,
// its recent window history (for sparklines), and its correctness findings.
type ContestantDetail struct {
	Entry      models.LeaderboardEntry  `json:"entry"`
	Rolling    *models.RollingStats     `json:"rolling,omitempty"`
	Validation *models.ValidationResult `json:"validation,omitempty"`
	History    []models.WindowAggregate `json:"history"`
}

// handleContestant returns the deep-dive for ?run=&submission=.
func (a *API) handleContestant(w http.ResponseWriter, r *http.Request) {
	run := r.URL.Query().Get("run")
	sub := r.URL.Query().Get("submission")
	if run == "" || sub == "" {
		http.Error(w, "missing ?run or ?submission", http.StatusBadRequest)
		return
	}
	entry, ok := a.svc.Entry(run, sub)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	detail := ContestantDetail{Entry: entry, History: []models.WindowAggregate{}}
	if a.pg != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		detail.History = a.queryHistory(ctx, run, sub)
		detail.Rolling = a.queryRolling(ctx, run, sub)
		detail.Validation = a.queryValidation(ctx, run, sub)
	}
	writeJSON(w, http.StatusOK, detail)
}

// handleWS upgrades to a WebSocket, subscribes the client to ?run=, and sends
// the current board immediately so the UI paints before the next update.
func (a *API) handleWS(w http.ResponseWriter, r *http.Request) {
	run := r.URL.Query().Get("run")
	if run == "" {
		http.Error(w, "missing ?run", http.StatusBadRequest)
		return
	}
	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &Client{hub: a.hub, run: run, conn: conn, send: make(chan []byte, clientSendBuffer)}
	a.hub.register(client)

	// Initial snapshot.
	if snap, err := json.Marshal(boardMessage{
		Type: "leaderboard", RunID: run, Entries: a.svc.Snapshot(run, Filter{}),
	}); err == nil {
		select {
		case client.send <- snap:
		default:
		}
	}
	go client.writePump()
	go client.readPump()
}

// ---- Postgres read queries for the contestant deep-dive ----

func (a *API) queryHistory(ctx context.Context, run, sub string) []models.WindowAggregate {
	const q = `
SELECT window_start, window_end, transactions, errors, timeouts, tps, error_rate,
       p50_us, p90_us, p99_us, max_us, mean_us, sample_count
FROM window_aggregates
WHERE run_id=$1 AND submission_id=$2 AND window_kind='tumbling'
ORDER BY window_start DESC
LIMIT 120;`
	rows, err := a.pg.Pool.Query(ctx, q, run, sub)
	if err != nil {
		a.log.Warn("history query", logpkg.F("err", err.Error()))
		return []models.WindowAggregate{}
	}
	defer rows.Close()
	out := []models.WindowAggregate{}
	for rows.Next() {
		wa := models.WindowAggregate{RunID: run, SubmissionID: sub, WindowKind: "tumbling"}
		if err := rows.Scan(&wa.WindowStart, &wa.WindowEnd, &wa.Transactions, &wa.Errors,
			&wa.Timeouts, &wa.TPS, &wa.ErrorRate, &wa.P50US, &wa.P90US, &wa.P99US,
			&wa.MaxUS, &wa.MeanUS, &wa.SampleCount); err != nil {
			continue
		}
		out = append(out, wa)
	}
	// Reverse to chronological order for charting.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func (a *API) queryRolling(ctx context.Context, run, sub string) *models.RollingStats {
	const q = `
SELECT updated_at, total_transactions, total_errors, peak_tps, current_tps,
       p50_us, p90_us, p99_us, error_rate, tps_stddev, tps_cov
FROM rolling_stats WHERE run_id=$1 AND submission_id=$2;`
	rs := &models.RollingStats{RunID: run, SubmissionID: sub}
	err := a.pg.Pool.QueryRow(ctx, q, run, sub).Scan(
		&rs.UpdatedAt, &rs.TotalTransactions, &rs.TotalErrors, &rs.PeakTPS, &rs.CurrentTPS,
		&rs.P50US, &rs.P90US, &rs.P99US, &rs.ErrorRate, &rs.TPSStdDev, &rs.TPSCoV)
	if err != nil {
		return nil
	}
	return rs
}

func (a *API) queryValidation(ctx context.Context, run, sub string) *models.ValidationResult {
	const q = `
SELECT updated_at, orders_checked, trades_checked, violations,
       violations_by_rule, correctness_score, recent_findings
FROM validation_results WHERE run_id=$1 AND submission_id=$2;`
	vr := &models.ValidationResult{RunID: run, SubmissionID: sub}
	var byRule, findings []byte
	err := a.pg.Pool.QueryRow(ctx, q, run, sub).Scan(
		&vr.UpdatedAt, &vr.OrdersChecked, &vr.TradesChecked, &vr.Violations,
		&byRule, &vr.CorrectnessScore, &findings)
	if err != nil {
		return nil
	}
	_ = json.Unmarshal(byRule, &vr.ViolationsByRule)
	_ = json.Unmarshal(findings, &vr.RecentFindings)
	return vr
}

// withCORS allows the dashboard (served from a different origin in dev) to read
// these endpoints from the browser.
func (a *API) withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
