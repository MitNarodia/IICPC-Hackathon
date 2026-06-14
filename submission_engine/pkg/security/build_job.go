package security

import "fmt"

type BuildJobRequest struct {
	Name                  string
	Namespace             string
	BuilderImage          string
	ServiceAccountName    string
	ActiveDeadlineSeconds int
	TTLSecondsAfterFinish int
	CPULimit              string
	MemoryLimit           string
}

func BuildRootlessJob(req BuildJobRequest) map[string]interface{} {
	if req.Namespace == "" {
		req.Namespace = "track1-build"
	}
	if req.BuilderImage == "" {
		req.BuilderImage = "moby/buildkit:rootless"
	}
	if req.ActiveDeadlineSeconds == 0 {
		req.ActiveDeadlineSeconds = 600
	}
	if req.TTLSecondsAfterFinish == 0 {
		req.TTLSecondsAfterFinish = 300
	}
	if req.CPULimit == "" {
		req.CPULimit = "2"
	}
	if req.MemoryLimit == "" {
		req.MemoryLimit = "2048Mi"
	}
	automount := false
	return map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      req.Name,
			"namespace": req.Namespace,
			"labels": map[string]string{
				"app":      "build-job",
				"workload": "build",
			},
		},
		"spec": map[string]interface{}{
			"backoffLimit":            1,
			"activeDeadlineSeconds":   req.ActiveDeadlineSeconds,
			"ttlSecondsAfterFinished": req.TTLSecondsAfterFinish,
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"restartPolicy":                "Never",
					"automountServiceAccountToken": automount,
					"nodeSelector": map[string]string{
						"workload": "build",
					},
					"tolerations": []map[string]string{{
						"key":      "workload",
						"operator": "Equal",
						"value":    "build",
						"effect":   "NoSchedule",
					}},
					"containers": []map[string]interface{}{{
						"name":  "builder",
						"image": req.BuilderImage,
						"securityContext": map[string]interface{}{
							"runAsNonRoot":             true,
							"allowPrivilegeEscalation": false,
							"privileged":               false,
							"readOnlyRootFilesystem":   true,
							"capabilities": map[string][]string{
								"drop": []string{"ALL"},
							},
						},
						"resources": map[string]interface{}{
							"requests": map[string]string{"cpu": req.CPULimit, "memory": req.MemoryLimit},
							"limits":   map[string]string{"cpu": req.CPULimit, "memory": req.MemoryLimit},
						},
					}},
				},
			},
		},
	}
}

func BuildJobName(submissionID string) string {
	return fmt.Sprintf("build-%s", SandboxName(submissionID))
}
