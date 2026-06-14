package security

type ObjectMeta struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type LabelSelector struct {
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

type LocalObjectReference struct {
	Name string `json:"name"`
}

type Toleration struct {
	Key      string `json:"key"`
	Operator string `json:"operator"`
	Value    string `json:"value,omitempty"`
	Effect   string `json:"effect,omitempty"`
}

type SeccompProfileRef struct {
	Type             string `json:"type"`
	LocalhostProfile string `json:"localhostProfile,omitempty"`
}

type PodSecurityContext struct {
	RunAsNonRoot   bool              `json:"runAsNonRoot"`
	RunAsUser      int64             `json:"runAsUser,omitempty"`
	RunAsGroup     int64             `json:"runAsGroup,omitempty"`
	FSGroup        int64             `json:"fsGroup,omitempty"`
	SeccompProfile SeccompProfileRef `json:"seccompProfile"`
}

type Capabilities struct {
	Drop []string `json:"drop,omitempty"`
	Add  []string `json:"add,omitempty"`
}

type ContainerSecurityContext struct {
	Privileged               bool         `json:"privileged"`
	AllowPrivilegeEscalation bool         `json:"allowPrivilegeEscalation"`
	ReadOnlyRootFilesystem   bool         `json:"readOnlyRootFilesystem"`
	RunAsNonRoot             bool         `json:"runAsNonRoot"`
	RunAsUser                int64        `json:"runAsUser"`
	Capabilities             Capabilities `json:"capabilities"`
}

type ResourceRequirements struct {
	Requests map[string]string `json:"requests"`
	Limits   map[string]string `json:"limits"`
}

type ContainerPort struct {
	Name          string `json:"name,omitempty"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol,omitempty"`
}

type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
}

type HTTPGetAction struct {
	Path string `json:"path"`
	Port int    `json:"port"`
}

type TCPSocketAction struct {
	Port int `json:"port"`
}

type Probe struct {
	HTTPGet             *HTTPGetAction   `json:"httpGet,omitempty"`
	TCPSocket           *TCPSocketAction `json:"tcpSocket,omitempty"`
	InitialDelaySeconds int              `json:"initialDelaySeconds,omitempty"`
	PeriodSeconds       int              `json:"periodSeconds,omitempty"`
	TimeoutSeconds      int              `json:"timeoutSeconds,omitempty"`
	FailureThreshold    int              `json:"failureThreshold,omitempty"`
}

type Container struct {
	Name            string                   `json:"name"`
	Image           string                   `json:"image"`
	ImagePullPolicy string                   `json:"imagePullPolicy,omitempty"`
	Command         []string                 `json:"command,omitempty"`
	Args            []string                 `json:"args,omitempty"`
	Ports           []ContainerPort          `json:"ports,omitempty"`
	Env             []EnvVar                 `json:"env,omitempty"`
	Resources       ResourceRequirements     `json:"resources"`
	SecurityContext ContainerSecurityContext `json:"securityContext"`
	VolumeMounts    []VolumeMount            `json:"volumeMounts,omitempty"`
	ReadinessProbe  *Probe                   `json:"readinessProbe,omitempty"`
	LivenessProbe   *Probe                   `json:"livenessProbe,omitempty"`
}

type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type EmptyDirVolumeSource struct {
	Medium    string `json:"medium,omitempty"`
	SizeLimit string `json:"sizeLimit,omitempty"`
}

type Volume struct {
	Name     string                `json:"name"`
	EmptyDir *EmptyDirVolumeSource `json:"emptyDir,omitempty"`
}

type PodSpec struct {
	RuntimeClassName             string                 `json:"runtimeClassName"`
	NodeSelector                 map[string]string      `json:"nodeSelector,omitempty"`
	Tolerations                  []Toleration           `json:"tolerations,omitempty"`
	AutomountServiceAccountToken *bool                  `json:"automountServiceAccountToken"`
	RestartPolicy                string                 `json:"restartPolicy,omitempty"`
	SecurityContext              PodSecurityContext     `json:"securityContext"`
	ImagePullSecrets             []LocalObjectReference `json:"imagePullSecrets,omitempty"`
	Containers                   []Container            `json:"containers"`
	Volumes                      []Volume               `json:"volumes,omitempty"`
}

type Pod struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       PodSpec    `json:"spec"`
}

type ServicePort struct {
	Name       string `json:"name,omitempty"`
	Port       int    `json:"port"`
	TargetPort int    `json:"targetPort"`
	Protocol   string `json:"protocol,omitempty"`
}

type ServiceSpec struct {
	Type     string            `json:"type"`
	Selector map[string]string `json:"selector"`
	Ports    []ServicePort     `json:"ports"`
}

type Service struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Metadata   ObjectMeta  `json:"metadata"`
	Spec       ServiceSpec `json:"spec"`
}

type NetworkPolicyPort struct {
	Protocol string `json:"protocol,omitempty"`
	Port     int    `json:"port"`
}

type NetworkPolicyPeer struct {
	NamespaceSelector *LabelSelector `json:"namespaceSelector,omitempty"`
	PodSelector       *LabelSelector `json:"podSelector,omitempty"`
}

type NetworkPolicyIngressRule struct {
	From  []NetworkPolicyPeer `json:"from,omitempty"`
	Ports []NetworkPolicyPort `json:"ports,omitempty"`
}

type NetworkPolicyEgressRule struct {
	To    []NetworkPolicyPeer `json:"to,omitempty"`
	Ports []NetworkPolicyPort `json:"ports,omitempty"`
}

type NetworkPolicySpec struct {
	PodSelector LabelSelector              `json:"podSelector"`
	PolicyTypes []string                   `json:"policyTypes"`
	Ingress     []NetworkPolicyIngressRule `json:"ingress,omitempty"`
	Egress      []NetworkPolicyEgressRule  `json:"egress,omitempty"`
}

type NetworkPolicy struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   ObjectMeta        `json:"metadata"`
	Spec       NetworkPolicySpec `json:"spec"`
}
