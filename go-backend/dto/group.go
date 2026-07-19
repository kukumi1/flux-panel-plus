package dto

type GroupMemberDto struct {
	NodeId   int64  `json:"nodeId" binding:"required"`
	MemberIp string `json:"memberIp"`
	Priority int    `json:"priority"`
}

type GroupDto struct {
	Name       string           `json:"name" binding:"required"`
	Type       string           `json:"type" binding:"required"`
	SwitchBack int              `json:"switchBack"`
	Members    []GroupMemberDto `json:"members"`
}

type GroupUpdateDto struct {
	ID         int64            `json:"id" binding:"required"`
	Name       string           `json:"name"`
	SwitchBack *int             `json:"switchBack"`
	Members    []GroupMemberDto `json:"members"`
}
