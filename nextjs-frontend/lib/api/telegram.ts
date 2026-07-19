import { post } from './client';

export type TelegramStatus = {
  bound: boolean;
  enabled: boolean;
};

export const getTelegramStatus = () => post<TelegramStatus>('/user/telegram/status');
export const getTelegramBindCode = () => post<{ code: string }>('/user/telegram/bind-code');
export const unbindTelegram = () => post('/user/telegram/unbind');
