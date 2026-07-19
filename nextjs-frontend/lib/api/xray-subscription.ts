import { post } from './client';

export const getSubscriptionToken = () => post('/v/sub/token');
export const getSubscriptionLinks = (scope?: 'mine' | 'all') =>
  post(`/v/sub/links${scope === 'mine' ? '?scope=mine' : ''}`);
export const resetSubscriptionToken = () => post('/v/sub/reset');
