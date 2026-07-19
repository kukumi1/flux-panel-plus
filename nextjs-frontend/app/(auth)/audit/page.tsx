'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { getConnectionAuditList, type ConnectionAuditItem, type ConnectionAuditListParams } from '@/lib/api/audit';
import { getNodeList } from '@/lib/api/node';
import { useAuth } from '@/lib/hooks/use-auth';
import { useTranslation } from '@/lib/i18n';
import { Search, RotateCcw, Pause, Play } from 'lucide-react';
import { toast } from 'sonner';

type AuditFilters = {
  node: string;
  clientEmail: string;
  target: string;
  service: string;
  forwardId: string;
  protocol: string;
  timeRange: string;
};

const pageSize = 50;

const defaultFilters: AuditFilters = {
  node: 'all',
  clientEmail: '',
  target: '',
  service: '',
  forwardId: '',
  protocol: 'all',
  timeRange: '24h',
};

function parsePositiveInt(value: string) {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
}

function timeRangeToStart(range: string) {
  const now = Date.now();
  if (range === '1h') return now - 60 * 60 * 1000;
  if (range === '24h') return now - 24 * 60 * 60 * 1000;
  if (range === '7d') return now - 7 * 24 * 60 * 60 * 1000;
  return 0;
}

function formatBytes(bytes: number) {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  return `${value.toFixed(value >= 10 || index === 0 ? 0 : 1)} ${units[index]}`;
}

function formatDuration(ms: number) {
  if (!ms) return '-';
  if (ms < 1000) return `${ms}ms`;
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainSeconds = seconds % 60;
  if (minutes < 60) return `${minutes}m ${remainSeconds}s`;
  const hours = Math.floor(minutes / 60);
  const remainMinutes = minutes % 60;
  return `${hours}h ${remainMinutes}m`;
}

// x-ui/xray and sing-box audit come from access-log tailing, which carries no
// per-connection byte accounting; only GOST forwards report real traffic bytes.
const LOG_ONLY_AUDIT_SOURCES = new Set(['xray', 'singbox']);
function sourceHasTraffic(sourceType?: string) {
  return !LOG_ONLY_AUDIT_SOURCES.has((sourceType || '').toLowerCase());
}

function formatTime(ts: number) {
  if (!ts) return '-';
  return new Date(ts).toLocaleString();
}

function endpointLabel(audit: ConnectionAuditItem) {
  if (audit.targetAddr) return audit.targetAddr;
  if (audit.targetHost && audit.targetPort) return `${audit.targetHost}:${audit.targetPort}`;
  if (audit.targetHost) return audit.targetHost;
  return '-';
}

function pathLabel(audit: ConnectionAuditItem) {
  if (audit.routeName) return audit.routeName;
  if (audit.tunnelName) return audit.tunnelName;
  if (audit.forwardName) return audit.forwardName;
  if (audit.forwardId) return `#${audit.forwardId}`;
  return audit.serviceName || '-';
}

export default function AuditPage() {
  const { isAdmin } = useAuth();
  const { t } = useTranslation();
  const [filters, setFilters] = useState<AuditFilters>(defaultFilters);
  const [query, setQuery] = useState<AuditFilters>(defaultFilters);
  const [audits, setAudits] = useState<ConnectionAuditItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [nodes, setNodes] = useState<{ id: number; name: string }[]>([]);

  const totalPages = useMemo(() => Math.max(1, Math.ceil(total / pageSize)), [total]);

  useEffect(() => {
    getNodeList().then(res => { if (res.code === 0) setNodes(res.data || []); }).catch(() => {});
  }, []);

  const buildParams = useCallback((currentPage: number): ConnectionAuditListParams => {
    const startTime = timeRangeToStart(query.timeRange);
    return {
      clientEmail: query.clientEmail.trim(),
      target: query.target.trim(),
      service: query.service.trim(),
      forwardId: parsePositiveInt(query.forwardId),
      nodeId: query.node.startsWith('node-') ? parsePositiveInt(query.node.slice(5)) : 0,
      nodeName: query.node === 'xui' ? 'x-ui:DMIT' : '',
      protocol: query.protocol,
      startTime,
      endTime: Date.now(),
      page: currentPage,
      pageSize,
    };
  }, [query]);

  const loadData = useCallback(async (currentPage: number) => {
    setLoading(true);
    try {
      const res = await getConnectionAuditList(buildParams(currentPage));
      if (res.code === 0) {
        setAudits(res.data?.list || []);
        setTotal(res.data?.total || 0);
      } else {
        toast.error(res.msg || t('common.loadFailed'));
      }
    } catch {
      toast.error(t('common.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [buildParams, t]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    getConnectionAuditList(buildParams(page))
      .then((res) => {
        if (cancelled) return;
        if (res.code === 0) {
          setAudits(res.data?.list || []);
          setTotal(res.data?.total || 0);
        } else {
          toast.error(res.msg || t('common.loadFailed'));
        }
      })
      .catch(() => {
        if (!cancelled) toast.error(t('common.loadFailed'));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [buildParams, page, t]);
  useEffect(() => {
    if (!autoRefresh) return undefined;
    const timer = window.setInterval(() => {
      void loadData(page);
    }, 10000);
    return () => window.clearInterval(timer);
  }, [autoRefresh, loadData, page]);

  const handleSearch = () => {
    setPage(1);
    setQuery(filters);
  };

  const handleReset = () => {
    setFilters(defaultFilters);
    setQuery(defaultFilters);
    setPage(1);
  };

  if (!isAdmin) {
    return (
      <div className="flex h-64 items-center justify-center">
        <p className="text-muted-foreground">{t('common.noPermission')}</p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-2xl font-bold">{t('audit.title')}</h2>
          <p className="text-sm text-muted-foreground">{t('audit.description')}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => setAutoRefresh(prev => !prev)}>
            {autoRefresh ? <Pause className="mr-2 h-4 w-4" /> : <Play className="mr-2 h-4 w-4" />}
            {autoRefresh ? t('audit.autoRefreshOn') : t('audit.autoRefreshOff')}
          </Button>
          <Button variant="outline" onClick={() => void loadData(page)} disabled={loading}>
            <RotateCcw className="mr-2 h-4 w-4" />
            {t('common.refresh')}
          </Button>
        </div>
      </div>

      <Card>
        <CardContent className="grid gap-4 p-4 md:grid-cols-3 xl:grid-cols-7">
          <div className="space-y-2">
            <Label>{t('audit.node')}</Label>
            <Select value={filters.node} onValueChange={value => setFilters(prev => ({ ...prev, node: value }))}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('audit.allNodes')}</SelectItem>
                <SelectItem value="xui">x-ui:DMIT</SelectItem>
                {nodes.map(n => (
                  <SelectItem key={n.id} value={`node-${n.id}`}>{n.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>{t('audit.clientEmail')}</Label>
            <Input value={filters.clientEmail} onChange={e => setFilters(prev => ({ ...prev, clientEmail: e.target.value }))} placeholder="email" />
          </div>
          <div className="space-y-2">
            <Label>{t('audit.target')}</Label>
            <Input value={filters.target} onChange={e => setFilters(prev => ({ ...prev, target: e.target.value }))} placeholder="example.com:443" />
          </div>
          <div className="space-y-2">
            <Label>{t('audit.service')}</Label>
            <Input value={filters.service} onChange={e => setFilters(prev => ({ ...prev, service: e.target.value }))} placeholder="service" />
          </div>
          <div className="space-y-2">
            <Label>{t('audit.forwardId')}</Label>
            <Input value={filters.forwardId} onChange={e => setFilters(prev => ({ ...prev, forwardId: e.target.value }))} inputMode="numeric" placeholder="ID" />
          </div>
          <div className="space-y-2">
            <Label>{t('audit.protocol')}</Label>
            <Select value={filters.protocol} onValueChange={value => setFilters(prev => ({ ...prev, protocol: value }))}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('audit.allProtocols')}</SelectItem>
                <SelectItem value="tcp">TCP</SelectItem>
                <SelectItem value="udp">UDP</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>{t('audit.timeRange')}</Label>
            <Select value={filters.timeRange} onValueChange={value => setFilters(prev => ({ ...prev, timeRange: value }))}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="1h">{t('audit.lastHour')}</SelectItem>
                <SelectItem value="24h">{t('audit.lastDay')}</SelectItem>
                <SelectItem value="7d">{t('audit.lastWeek')}</SelectItem>
                <SelectItem value="all">{t('audit.allTime')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex gap-2 md:col-span-3 xl:col-span-7">
            <Button onClick={handleSearch} disabled={loading}>
              <Search className="mr-2 h-4 w-4" />
              {t('audit.search')}
            </Button>
            <Button variant="outline" onClick={handleReset} disabled={loading}>{t('audit.reset')}</Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('audit.time')}</TableHead>
                  <TableHead>{t('audit.client')}</TableHead>
                  <TableHead>{t('audit.destination')}</TableHead>
                  <TableHead>{t('audit.path')}</TableHead>
                  <TableHead>{t('audit.node')}</TableHead>
                  <TableHead>{t('audit.protocol')}</TableHead>
                  <TableHead>{t('audit.bytes')}</TableHead>
                  <TableHead>{t('audit.duration')}</TableHead>
                  <TableHead>{t('audit.result')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {loading ? (
                  <TableRow><TableCell colSpan={9} className="py-8 text-center text-muted-foreground">{t('common.loading')}</TableCell></TableRow>
                ) : audits.length === 0 ? (
                  <TableRow><TableCell colSpan={9} className="py-8 text-center text-muted-foreground">{t('common.noData')}</TableCell></TableRow>
                ) : audits.map(audit => (
                  <TableRow key={audit.id}>
                    <TableCell className="whitespace-nowrap text-sm">{formatTime(audit.endedTime || audit.createdTime)}</TableCell>
                    <TableCell>
                      <div className="font-medium">{audit.clientEmail || audit.clientIp || '-'}</div>
                      <div className="text-xs text-muted-foreground">{audit.clientEmail ? (audit.clientIp || '-') : (audit.clientPort || '-')}</div>
                    </TableCell>
                    <TableCell className="min-w-48">
                      <div className="font-medium">{endpointLabel(audit)}</div>
                      {audit.targetHost && <div className="text-xs text-muted-foreground">{audit.targetHost}</div>}
                    </TableCell>
                    <TableCell className="min-w-40">
                      <div className="font-medium">{pathLabel(audit)}</div>
                      <div className="text-xs text-muted-foreground">{audit.serviceName || '-'}</div>
                    </TableCell>
                    <TableCell>
                      <div className="font-medium">{audit.nodeName || '-'}</div>
                      <div className="text-xs text-muted-foreground">{audit.sourceType || (audit.nodeId ? `#${audit.nodeId}` : '-')}</div>
                    </TableCell>
                    <TableCell><Badge variant="outline" className="rounded-md uppercase">{audit.protocol || '-'}</Badge></TableCell>
                    <TableCell className="whitespace-nowrap text-sm">
                      {sourceHasTraffic(audit.sourceType) ? (
                        <>
                          <div>{t('audit.up')}: {formatBytes(audit.upBytes)}</div>
                          <div className="text-muted-foreground">{t('audit.down')}: {formatBytes(audit.downBytes)}</div>
                        </>
                      ) : (
                        <span className="text-muted-foreground">{t('audit.notApplicable')}</span>
                      )}
                    </TableCell>
                    <TableCell className="whitespace-nowrap">
                      {audit.durationMs
                        ? formatDuration(audit.durationMs)
                        : sourceHasTraffic(audit.sourceType)
                          ? '-'
                          : t('audit.notApplicable')}
                    </TableCell>
                    <TableCell>
                      {audit.error ? (
                        <Badge variant="destructive" className="max-w-48 rounded-md whitespace-normal">{audit.error}</Badge>
                      ) : (
                        <Badge variant="secondary" className="rounded-md">{t('audit.success')}</Badge>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </CardContent>
      </Card>

      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-sm text-muted-foreground">
          {t('audit.total', { total, page, totalPages })}
        </p>
        <div className="flex gap-2">
          <Button variant="outline" disabled={loading || page <= 1} onClick={() => setPage(prev => Math.max(1, prev - 1))}>
            {t('audit.previous')}
          </Button>
          <Button variant="outline" disabled={loading || page >= totalPages} onClick={() => setPage(prev => Math.min(totalPages, prev + 1))}>
            {t('audit.next')}
          </Button>
        </div>
      </div>
    </div>
  );
}