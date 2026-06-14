package security

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	DefaultSandboxNamespace = "track1-sandbox"
	DefaultRuntimeClass     = "gvisor"
	DefaultAppArmorProfile  = "track1-sandbox"
	DefaultSeccompProfile   = "seccomp/track1-sandbox.json"
	DefaultRunAsUser        = int64(65532)
	DefaultScratchSize      = "64Mi"
)

type SandboxPodRequest struct {
	SubmissionID  string
	ImageRef      string
	Entrypoint    string
	DeclaredPort  int
	HealthPath    string
	CPUCores      int
	MemoryMB      int
	Namespace     string
	RuntimeClass  string
	NodeSelector  map[string]string
	TolerationKey string
	TolerationVal string
}

type SandboxManifests struct {
	Pod             Pod
	Service         Service
	NetworkPolicies []NetworkPolicy
}

func BuildSandboxManifests(req SandboxPodRequest) (SandboxManifests, error) {
	pod, err := BuildSandboxPod(req)
	if err != nil {
		return SandboxManifests{}, err
	}
	service, err := BuildSandboxService(req)
	if err != nil {
		return SandboxManifests{}, err
	}
	netpols, err := BuildSandboxNetworkPolicies(req)
	if err != nil {
		return SandboxManifests{}, err
	}
	return SandboxManifests{Pod: pod, Service: service, NetworkPolicies: netpols}, nil
}

func BuildSandboxPod(req SandboxPodRequest) (Pod, error) {
	req = withSandboxDefaults(req)
	if err := validateSandboxRequest(req); err != nil {
		return Pod{}, err
	}
	name := SandboxName(req.SubmissionID)
	automount := false
	labels := sandboxLabels(req.SubmissionID)
	cpu := strconv.Itoa(req.CPUCores)
	memory := fmt.Sprintf("%dMi", req.MemoryMB)
	healthPath := req.HealthPath
	if healthPath == "" {
		healthPath = "/healthz"
	}

	pod := Pod{
		APIVersion: "v1",
		Kind:       "Pod",
		Metadata: ObjectMeta{
			Name:      name,
			Namespace: req.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"container.apparmor.security.beta.kubernetes.io/submission": "localhost/" + DefaultAppArmorProfile,
			},
		},
		Spec: PodSpec{
			RuntimeClassName:             req.RuntimeClass,
			NodeSelector:                 req.NodeSelector,
			AutomountServiceAccountToken: &automount,
			RestartPolicy:                "Never",
			SecurityContext: PodSecurityContext{
				RunAsNonRoot: true,
				RunAsUser:    DefaultRunAsUser,
				RunAsGroup:   DefaultRunAsUser,
				FSGroup:      DefaultRunAsUser,
				SeccompProfile: SeccompProfileRef{
					Type:             "Localhost",
					LocalhostProfile: DefaultSeccompProfile,
				},
			},
			Tolerations: []Toleration{{
				Key:      req.TolerationKey,
				Operator: "Equal",
				Value:    req.TolerationVal,
				Effect:   "NoSchedule",
			}},
			Containers: []Container{{
				Name:            "submission",
				Image:           req.ImageRef,
				ImagePullPolicy: "IfNotPresent",
				Command:         commandFromEntrypoint(req.Entrypoint),
				Ports: []ContainerPort{
					{Name: "submission", ContainerPort: req.DeclaredPort, Protocol: "TCP"},
				},
				Resources: ResourceRequirements{
					Requests: map[string]string{"cpu": cpu, "memory": memory},
					Limits:   map[string]string{"cpu": cpu, "memory": memory},
				},
				SecurityContext: ContainerSecurityContext{
					Privileged:               false,
					AllowPrivilegeEscalation: false,
					ReadOnlyRootFilesystem:   true,
					RunAsNonRoot:             true,
					RunAsUser:                DefaultRunAsUser,
					Capabilities:             Capabilities{Drop: []string{"ALL"}},
				},
				VolumeMounts: []VolumeMount{
					{Name: "scratch", MountPath: "/tmp"},
				},
				ReadinessProbe: &Probe{
					HTTPGet:             &HTTPGetAction{Path: healthPath, Port: req.DeclaredPort},
					InitialDelaySeconds: 2,
					PeriodSeconds:       5,
					TimeoutSeconds:      2,
					FailureThreshold:    3,
				},
				LivenessProbe: &Probe{
					TCPSocket:        &TCPSocketAction{Port: req.DeclaredPort},
					PeriodSeconds:    10,
					TimeoutSeconds:   2,
					FailureThreshold: 3,
				},
			}},
			Volumes: []Volume{{
				Name:     "scratch",
				EmptyDir: &EmptyDirVolumeSource{Medium: "Memory", SizeLimit: DefaultScratchSize},
			}},
		},
	}
	return pod, nil
}

func BuildSandboxService(req SandboxPodRequest) (Service, error) {
	req = withSandboxDefaults(req)
	if err := validateSandboxRequest(req); err != nil {
		return Service{}, err
	}
	return Service{
		APIVersion: "v1",
		Kind:       "Service",
		Metadata: ObjectMeta{
			Name:      SandboxName(req.SubmissionID),
			Namespace: req.Namespace,
			Labels:    sandboxLabels(req.SubmissionID),
		},
		Spec: ServiceSpec{
			Type:     "ClusterIP",
			Selector: sandboxLabels(req.SubmissionID),
			Ports: []ServicePort{{
				Name:       "submission",
				Port:       req.DeclaredPort,
				TargetPort: req.DeclaredPort,
				Protocol:   "TCP",
			}},
		},
	}, nil
}

func BuildSandboxNetworkPolicies(req SandboxPodRequest) ([]NetworkPolicy, error) {
	req = withSandboxDefaults(req)
	if err := validateSandboxRequest(req); err != nil {
		return nil, err
	}
	name := SandboxName(req.SubmissionID)
	selector := LabelSelector{MatchLabels: sandboxLabels(req.SubmissionID)}
	defaultDeny := NetworkPolicy{
		APIVersion: "networking.k8s.io/v1",
		Kind:       "NetworkPolicy",
		Metadata: ObjectMeta{
			Name:      name + "-default-deny",
			Namespace: req.Namespace,
			Labels:    sandboxLabels(req.SubmissionID),
		},
		Spec: NetworkPolicySpec{
			PodSelector: selector,
			PolicyTypes: []string{"Ingress", "Egress"},
			Ingress:     []NetworkPolicyIngressRule{},
			Egress:      []NetworkPolicyEgressRule{},
		},
	}
	allowIngress := NetworkPolicy{
		APIVersion: "networking.k8s.io/v1",
		Kind:       "NetworkPolicy",
		Metadata: ObjectMeta{
			Name:      name + "-allow-ingress",
			Namespace: req.Namespace,
			Labels:    sandboxLabels(req.SubmissionID),
		},
		Spec: NetworkPolicySpec{
			PodSelector: selector,
			PolicyTypes: []string{"Ingress"},
			Ingress: []NetworkPolicyIngressRule{{
				From: []NetworkPolicyPeer{
					{NamespaceSelector: &LabelSelector{MatchLabels: map[string]string{"track": "track1-system"}}},
					{NamespaceSelector: &LabelSelector{MatchLabels: map[string]string{"track": "bot-fleet"}}},
				},
				Ports: []NetworkPolicyPort{{Protocol: "TCP", Port: req.DeclaredPort}},
			}},
		},
	}
	return []NetworkPolicy{defaultDeny, allowIngress}, nil
}

func ValidateSandboxPod(pod Pod) error {
	var problems []string
	if pod.Spec.RuntimeClassName != DefaultRuntimeClass {
		problems = append(problems, "runtimeClassName must be gvisor")
	}
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		problems = append(problems, "automountServiceAccountToken must be false")
	}
	if pod.Spec.SecurityContext.SeccompProfile.Type != "Localhost" {
		problems = append(problems, "seccompProfile must be Localhost")
	}
	if pod.Spec.NodeSelector["sandbox"] != "true" {
		problems = append(problems, "nodeSelector sandbox=true is required")
	}
	if len(pod.Spec.Containers) != 1 {
		problems = append(problems, "exactly one submission container is required")
	}
	if len(pod.Spec.Containers) == 1 {
		c := pod.Spec.Containers[0]
		if c.SecurityContext.Privileged {
			problems = append(problems, "privileged must be false")
		}
		if c.SecurityContext.AllowPrivilegeEscalation {
			problems = append(problems, "allowPrivilegeEscalation must be false")
		}
		if !c.SecurityContext.ReadOnlyRootFilesystem {
			problems = append(problems, "readOnlyRootFilesystem must be true")
		}
		if !contains(c.SecurityContext.Capabilities.Drop, "ALL") || len(c.SecurityContext.Capabilities.Add) > 0 {
			problems = append(problems, "capabilities must drop ALL and add none")
		}
		if c.Resources.Requests["cpu"] != c.Resources.Limits["cpu"] || c.Resources.Requests["memory"] != c.Resources.Limits["memory"] {
			problems = append(problems, "requests must equal limits for Guaranteed QoS")
		}
		if _, err := strconv.Atoi(c.Resources.Limits["cpu"]); err != nil {
			problems = append(problems, "cpu limit must be an integer core count")
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func ToJSON(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func SandboxName(submissionID string) string {
	id := strings.ToLower(strings.ReplaceAll(submissionID, "_", "-"))
	if len(id) > 48 {
		id = id[:48]
	}
	return "submission-" + id
}

func withSandboxDefaults(req SandboxPodRequest) SandboxPodRequest {
	if req.Namespace == "" {
		req.Namespace = DefaultSandboxNamespace
	}
	if req.RuntimeClass == "" {
		req.RuntimeClass = DefaultRuntimeClass
	}
	if req.CPUCores == 0 {
		req.CPUCores = 1
	}
	if req.MemoryMB == 0 {
		req.MemoryMB = 512
	}
	if req.NodeSelector == nil {
		req.NodeSelector = map[string]string{"sandbox": "true"}
	}
	if req.TolerationKey == "" {
		req.TolerationKey = "workload"
	}
	if req.TolerationVal == "" {
		req.TolerationVal = "untrusted"
	}
	return req
}

func validateSandboxRequest(req SandboxPodRequest) error {
	if req.SubmissionID == "" {
		return errors.New("submission_id is required")
	}
	if req.ImageRef == "" {
		return errors.New("image_ref is required")
	}
	if !strings.Contains(req.ImageRef, "@sha256:") {
		return errors.New("image_ref must be pinned by digest")
	}
	if req.DeclaredPort < 1024 || req.DeclaredPort > 65535 {
		return errors.New("declared_port must be in [1024,65535]")
	}
	if req.CPUCores <= 0 {
		return errors.New("cpu_cores must be a positive integer")
	}
	if req.MemoryMB <= 0 {
		return errors.New("memory_mb must be positive")
	}
	if req.RuntimeClass != DefaultRuntimeClass {
		return errors.New("runtime_class must be gvisor")
	}
	if req.NodeSelector["sandbox"] != "true" {
		return errors.New("nodeSelector sandbox=true is required")
	}
	return nil
}

func sandboxLabels(submissionID string) map[string]string {
	return map[string]string{
		"app":           "sandbox",
		"sandbox":       "true",
		"submission_id": strings.ToLower(submissionID),
	}
}

func commandFromEntrypoint(entrypoint string) []string {
	entrypoint = strings.TrimSpace(entrypoint)
	if entrypoint == "" {
		return nil
	}
	return strings.Fields(entrypoint)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
