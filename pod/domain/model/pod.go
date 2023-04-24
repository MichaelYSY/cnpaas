package model

type Pod struct {
	ID            int64     `gorm:"primary_key;not_null;auto_increment" json:"id"`
	PodName       string    `gorm:"unique_index;not_null" json:"pod_name"`
	PodNamespace  string    `json:"pod_namespace"`
	PodTeamID     int64     `json:"pod_team_id"`
	PodCpuMin     float32   `json:"pod_cpu_min"`
	PodCpuMax     float32   `json:"pod_cpu_max"`
	PodRelicas    int32     `json:"pod_relicas"`
	PodMemoryMin  float32   `json:"pod_memory_min"`
	PodMemoryMax  float32   `json:"pod_memory_max"`
	PodPort       []PodPort `gorm:"ForeignKey:PodID" json:"pod_port"`
	PodEnv        []PodEnv  `gorm:"ForeignKey:PodID" json:"pod_env"`
	PodPullPolicy string    `json:"pod_pull_policy"`
	PodRestart    string    `json:"pod_restart"`
	PodType       string    `json:"pod_type"`
	PodImage      string    `json:"pod_image"`
}
