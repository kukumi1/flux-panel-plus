import { post } from './client';

export type DdnsProvider = {
  id: number;
  name: string;
  type: string;
  createdTime: number;
  updatedTime: number;
};

export type DdnsDomain = {
  id: number;
  providerId: number;
  groupId: number;
  domain: string;
  recordName: string;
  recordType: string;
  autoResolve: number;
  currentRecord: string;
  currentMemberId: number;
  createdTime: number;
  updatedTime: number;
  status: number;
};

export type DdnsProviderCreatePayload = {
  name: string;
  type: string;
  credential: unknown;
};

export type DdnsProviderUpdatePayload = {
  id: number;
  name?: string;
  credential?: unknown;
};

export type DdnsDomainCreatePayload = {
  providerId: number;
  groupId: number;
  domain: string;
  recordName?: string;
  recordType: string;
  autoResolve: number;
};

export type DdnsDomainUpdatePayload = {
  id: number;
  providerId?: number;
  domain?: string;
  recordName?: string;
  recordType?: string;
  autoResolve?: number;
};

export const getDdnsProviderTypes = () => post<string[]>('/ddns/provider/types');
export const getDdnsProviderList = () => post<DdnsProvider[]>('/ddns/provider/list');
export const createDdnsProvider = (data: DdnsProviderCreatePayload) => post('/ddns/provider/create', data);
export const updateDdnsProvider = (data: DdnsProviderUpdatePayload) => post('/ddns/provider/update', data);
export const deleteDdnsProvider = (id: number) => post('/ddns/provider/delete', { id });

export const getDdnsDomainList = () => post<DdnsDomain[]>('/ddns/domain/list');
export const createDdnsDomain = (data: DdnsDomainCreatePayload) => post('/ddns/domain/create', data);
export const updateDdnsDomain = (data: DdnsDomainUpdatePayload) => post('/ddns/domain/update', data);
export const deleteDdnsDomain = (id: number) => post('/ddns/domain/delete', { id });
export const resolveDdnsDomain = (id: number) => post('/ddns/domain/resolve', { id });
