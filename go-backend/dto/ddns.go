package dto

import "encoding/json"

type DdnsProviderDto struct {
	Name       string          `json:"name" binding:"required"`
	Type       string          `json:"type" binding:"required"`
	Credential json.RawMessage `json:"credential" binding:"required"`
}

type DdnsProviderUpdateDto struct {
	ID         int64           `json:"id" binding:"required"`
	Name       string          `json:"name"`
	Credential json.RawMessage `json:"credential"`
}

type DdnsDomainDto struct {
	ProviderId  int64  `json:"providerId" binding:"required"`
	GroupId     int64  `json:"groupId" binding:"required"`
	Domain      string `json:"domain" binding:"required"`
	RecordName  string `json:"recordName"`
	RecordType  string `json:"recordType" binding:"required"`
	AutoResolve int    `json:"autoResolve"`
}

type DdnsDomainUpdateDto struct {
	ID          int64  `json:"id" binding:"required"`
	ProviderId  int64  `json:"providerId"`
	Domain      string `json:"domain"`
	RecordName  string `json:"recordName"`
	RecordType  string `json:"recordType"`
	AutoResolve *int   `json:"autoResolve"`
}
