import { post } from './client';

export type ConnectionAuditItem = {
  id: number;
  nodeId: number;
  nodeName?: string;
  serviceName?: string;
  forwardId: number;
  forwardName?: string;
  routeId: number;
  routeName?: string;
  tunnelId: number;
  tunnelName?: string;
  userId: number;
  userName?: string;
  clientAddr?: string;
  clientIp?: string;
  clientEmail?: string;
  clientPort: number;
  sourceType?: string;
  targetHost?: string;
  targetPort: number;
  targetAddr?: string;
  protocol?: string;
  upBytes: number;
  downBytes: number;
  durationMs: number;
  startedTime: number;
  endedTime: number;
  error?: string;
  createdTime: number;
};

export type ConnectionAuditListParams = {
  clientIp?: string;
  clientEmail?: string;
  target?: string;
  service?: string;
  forwardId?: number;
  nodeId?: number;
  nodeName?: string;
  protocol?: string;
  startTime?: number;
  endTime?: number;
  page?: number;
  pageSize?: number;
};

export type ConnectionAuditListResult = {
  list: ConnectionAuditItem[];
  total: number;
  page: number;
  pageSize: number;
};

export const getConnectionAuditList = (data: ConnectionAuditListParams) =>
  post<ConnectionAuditListResult>('/audit/list', data);