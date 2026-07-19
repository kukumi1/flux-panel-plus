import { post } from './client';

type RoutePayload = {
  name: string;
  nodeIds: number[];
  protocol?: string;
  tcpListenAddr?: string;
  udpListenAddr?: string;
  interfaceName?: string;
  status?: number;
};

type RouteUpdatePayload = RoutePayload & {
  id: number;
};

export const createRoute = (data: RoutePayload) => post('/route/create', data);
export const getRouteList = () => post('/route/list');
export const updateRoute = (data: RouteUpdatePayload) => post('/route/update', data);
export const deleteRoute = (id: number) => post('/route/delete', { id });
