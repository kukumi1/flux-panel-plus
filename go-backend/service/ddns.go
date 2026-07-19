package service

import (
	"flux-panel/go-backend/config"
	"flux-panel/go-backend/dto"
	"flux-panel/go-backend/model"
	"flux-panel/go-backend/pkg"
	"flux-panel/go-backend/pkg/ddns"
	"fmt"
	"time"
)

func ddnsCrypto() *pkg.AESCrypto {
	return pkg.GetOrCreateCrypto(config.Cfg.JWTSecret)
}

func CreateDdnsProvider(d dto.DdnsProviderDto) dto.R {
	if _, err := ddns.New(d.Type, string(d.Credential)); err != nil {
		return dto.Err(err.Error())
	}

	crypto := ddnsCrypto()
	if crypto == nil {
		return dto.Err("加密密钥未配置，无法保存凭据")
	}
	encrypted, err := crypto.Encrypt(string(d.Credential))
	if err != nil {
		return dto.Err("凭据加密失败")
	}

	now := time.Now().UnixMilli()
	provider := model.DdnsProvider{
		Name:        d.Name,
		Type:        d.Type,
		Credential:  encrypted,
		Status:      1,
		CreatedTime: now,
		UpdatedTime: now,
	}
	if err := DB.Create(&provider).Error; err != nil {
		return dto.Err("创建服务商失败")
	}
	return dto.Ok(provider)
}

func GetAllDdnsProviders() dto.R {
	var providers []model.DdnsProvider
	DB.Order("created_time DESC").Find(&providers)

	result := make([]map[string]interface{}, 0, len(providers))
	for _, p := range providers {
		result = append(result, map[string]interface{}{
			"id":          p.ID,
			"name":        p.Name,
			"type":        p.Type,
			"createdTime": p.CreatedTime,
			"updatedTime": p.UpdatedTime,
		})
	}
	return dto.Ok(result)
}

func UpdateDdnsProvider(d dto.DdnsProviderUpdateDto) dto.R {
	var provider model.DdnsProvider
	if err := DB.First(&provider, d.ID).Error; err != nil {
		return dto.Err("服务商不存在")
	}

	if d.Name != "" {
		provider.Name = d.Name
	}
	if len(d.Credential) > 0 {
		if _, err := ddns.New(provider.Type, string(d.Credential)); err != nil {
			return dto.Err(err.Error())
		}
		crypto := ddnsCrypto()
		if crypto == nil {
			return dto.Err("加密密钥未配置，无法保存凭据")
		}
		encrypted, err := crypto.Encrypt(string(d.Credential))
		if err != nil {
			return dto.Err("凭据加密失败")
		}
		provider.Credential = encrypted
	}
	provider.UpdatedTime = time.Now().UnixMilli()
	DB.Save(&provider)
	return dto.Ok("更新成功")
}

func DeleteDdnsProvider(id int64) dto.R {
	var count int64
	DB.Model(&model.DdnsDomain{}).Where("provider_id = ?", id).Count(&count)
	if count > 0 {
		return dto.Err("该服务商下仍有域名，无法删除")
	}
	DB.Delete(&model.DdnsProvider{}, id)
	return dto.Ok("删除成功")
}

func CreateDdnsDomain(d dto.DdnsDomainDto) dto.R {
	if d.RecordType != "A" && d.RecordType != "AAAA" {
		return dto.Err("记录类型仅支持 A / AAAA")
	}
	if err := DB.First(&model.DdnsProvider{}, d.ProviderId).Error; err != nil {
		return dto.Err("服务商不存在")
	}
	if err := DB.First(&model.NodeGroup{}, d.GroupId).Error; err != nil {
		return dto.Err("分组不存在")
	}

	now := time.Now().UnixMilli()
	domain := model.DdnsDomain{
		ProviderId:  d.ProviderId,
		GroupId:     d.GroupId,
		Domain:      d.Domain,
		RecordName:  d.RecordName,
		RecordType:  d.RecordType,
		AutoResolve: d.AutoResolve,
		Status:      1,
		CreatedTime: now,
		UpdatedTime: now,
	}
	if err := DB.Create(&domain).Error; err != nil {
		return dto.Err("创建域名失败")
	}
	return dto.Ok(domain)
}

func GetAllDdnsDomains() dto.R {
	var domains []model.DdnsDomain
	DB.Order("created_time DESC").Find(&domains)
	return dto.Ok(domains)
}

func UpdateDdnsDomain(d dto.DdnsDomainUpdateDto) dto.R {
	var domain model.DdnsDomain
	if err := DB.First(&domain, d.ID).Error; err != nil {
		return dto.Err("域名不存在")
	}

	if d.ProviderId != 0 {
		domain.ProviderId = d.ProviderId
	}
	if d.Domain != "" {
		domain.Domain = d.Domain
	}
	if d.RecordName != "" {
		domain.RecordName = d.RecordName
	}
	if d.RecordType != "" {
		if d.RecordType != "A" && d.RecordType != "AAAA" {
			return dto.Err("记录类型仅支持 A / AAAA")
		}
		domain.RecordType = d.RecordType
	}
	if d.AutoResolve != nil {
		domain.AutoResolve = *d.AutoResolve
	}
	domain.UpdatedTime = time.Now().UnixMilli()
	DB.Save(&domain)
	return dto.Ok("更新成功")
}

func DeleteDdnsDomain(id int64) dto.R {
	DB.Delete(&model.DdnsDomain{}, id)
	return dto.Ok("删除成功")
}

// ResolveDdnsDomain manually points a domain at its group's active member IP.
func ResolveDdnsDomain(id int64) dto.R {
	var domain model.DdnsDomain
	if err := DB.First(&domain, id).Error; err != nil {
		return dto.Err("域名不存在")
	}
	member, ok := activeGroupMember(domain.GroupId)
	if !ok {
		return dto.Err("分组当前无可用成员")
	}
	if err := applyDdnsRecord(&domain, member); err != nil {
		return dto.Err(err.Error())
	}
	return dto.Ok("解析已更新")
}

// providerFor builds a live DNS provider client from a stored provider row,
// decrypting its credential.
func providerFor(providerId int64) (ddns.Provider, error) {
	var p model.DdnsProvider
	if err := DB.First(&p, providerId).Error; err != nil {
		return nil, fmt.Errorf("服务商不存在")
	}
	crypto := ddnsCrypto()
	if crypto == nil {
		return nil, fmt.Errorf("加密密钥未配置")
	}
	credential, err := crypto.Decrypt(p.Credential)
	if err != nil {
		return nil, fmt.Errorf("凭据解密失败")
	}
	return ddns.New(p.Type, credential)
}

// applyDdnsRecord pushes the member's IP to the DNS provider and records the
// result on the domain row plus the system log.
func applyDdnsRecord(domain *model.DdnsDomain, member model.GroupMember) error {
	ip := memberIP(member)
	if ip == "" {
		return fmt.Errorf("成员无可用 IP")
	}

	provider, err := providerFor(domain.ProviderId)
	if err != nil {
		return err
	}
	if err := provider.SetRecord(domain.Domain, domain.RecordName, domain.RecordType, ip); err != nil {
		WriteSystemLog("ddns", "error", fmt.Sprintf("更新 %s 失败: %v", ddnsLabel(domain), err))
		return fmt.Errorf("DNS 更新失败: %v", err)
	}

	domain.CurrentRecord = ip
	domain.CurrentMemberId = member.NodeId
	domain.UpdatedTime = time.Now().UnixMilli()
	DB.Save(domain)

	WriteSystemLog("ddns", "info", fmt.Sprintf("%s → %s", ddnsLabel(domain), ip))
	return nil
}

func ddnsLabel(domain *model.DdnsDomain) string {
	if domain.RecordName == "" || domain.RecordName == "@" {
		return domain.Domain
	}
	return domain.RecordName + "." + domain.Domain
}
