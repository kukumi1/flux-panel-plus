import { post } from './client';

export type GroupMember = {
  id?: number;
  nodeId: number;
  nodeName?: string;
  memberIp?: string;
  priority: number;
  online?: boolean;
};

export type NodeGroup = {
  id: number;
  name: string;
  type: string;
  switchBack: number;
  activeMemberId: number;
  lastSwitchTime: number;
  inx: number;
  members: GroupMember[];
};

export type GroupMemberPayload = {
  nodeId: number;
  memberIp?: string;
  priority: number;
};

export type GroupCreatePayload = {
  name: string;
  type: string;
  switchBack: number;
  members: GroupMemberPayload[];
};

export type GroupUpdatePayload = {
  id: number;
  name?: string;
  switchBack?: number;
  members?: GroupMemberPayload[];
};

export const getGroupList = () => post<NodeGroup[]>('/group/list');
export const createGroup = (data: GroupCreatePayload) => post('/group/create', data);
export const updateGroup = (data: GroupUpdatePayload) => post('/group/update', data);
export const deleteGroup = (id: number) => post('/group/delete', { id });
export const updateGroupOrder = (items: { id: number; inx: number }[]) => post('/group/update-order', items);
