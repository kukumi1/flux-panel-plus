import { post } from './client';

export type SystemLog = {
  id: number;
  type: string;
  level: string;
  message: string;
  createdTime: number;
};

export const getSystemLogs = (params: { type?: string; limit?: number } = {}) =>
  post<SystemLog[]>('/system-log/list', params);
