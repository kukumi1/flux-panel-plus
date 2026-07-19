package dto

type ConnectionAuditUploadDto struct {
	Events []ConnectionAuditEventDto `json:"events"`
}

type ConnectionAuditEventDto struct {
	ServiceName string `json:"serviceName"`
	ClientAddr  string `json:"clientAddr"`
	ClientEmail string `json:"clientEmail"`
	TargetAddr  string `json:"targetAddr"`
	Protocol    string `json:"protocol"`
	UpBytes     int64  `json:"upBytes"`
	DownBytes   int64  `json:"downBytes"`
	DurationMs  int64  `json:"durationMs"`
	StartedTime int64  `json:"startedTime"`
	EndedTime   int64  `json:"endedTime"`
	Error       string `json:"error"`
}

type ConnectionAuditListDto struct {
	ClientIp    string `json:"clientIp"`
	ClientEmail string `json:"clientEmail"`
	Target      string `json:"target"`
	Service     string `json:"service"`
	ForwardId int64  `json:"forwardId"`
	NodeId    int64  `json:"nodeId"`
	NodeName  string `json:"nodeName"`
	Protocol  string `json:"protocol"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
	Page      int    `json:"page"`
	PageSize  int    `json:"pageSize"`
}
