'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Plus, Edit2, Trash2, X } from 'lucide-react';
import { toast } from 'sonner';
import { createRoute, deleteRoute, getRouteList, updateRoute } from '@/lib/api/route';
import { getNodeList } from '@/lib/api/node';
import { useAuth } from '@/lib/hooks/use-auth';
import { useTranslation } from '@/lib/i18n';

type NodeItem = {
  id: number;
  name: string;
  serverIp?: string;
};

type RouteHop = {
  hopOrder: number;
  nodeId: number;
  nodeIp?: string;
  nodeName?: string;
};

type RouteItem = {
  id: number;
  name: string;
  protocol?: string;
  tcpListenAddr?: string;
  udpListenAddr?: string;
  interfaceName?: string;
  status?: number;
  hops?: RouteHop[];
  latency?: number | null;
};

type RouteForm = {
  name: string;
  nodeIds: string[];
  protocol: string;
  tcpListenAddr: string;
  udpListenAddr: string;
  interfaceName: string;
  status: string;
};

const emptyForm: RouteForm = {
  name: '',
  nodeIds: ['', ''],
  protocol: 'tls',
  tcpListenAddr: '::',
  udpListenAddr: '::',
  interfaceName: '',
  status: '1',
};

export default function RoutePage() {
  const { isAdmin } = useAuth();
  const { t } = useTranslation();
  const [routes, setRoutes] = useState<RouteItem[]>([]);
  const [nodes, setNodes] = useState<NodeItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingRoute, setEditingRoute] = useState<RouteItem | null>(null);
  const [form, setForm] = useState<RouteForm>(emptyForm);

  const nodeById = useMemo(() => {
    const map = new Map<number, NodeItem>();
    for (const node of nodes) map.set(node.id, node);
    return map;
  }, [nodes]);

  const loadData = useCallback(async () => {
    try {
      const [routeRes, nodeRes] = await Promise.all([getRouteList(), getNodeList()]);
      if (routeRes.code === 0) setRoutes(routeRes.data || []);
      if (nodeRes.code === 0) setNodes(nodeRes.data || []);
    } catch {
      toast.error(t('common.loadFailed'));
    }
    setLoading(false);
  }, [t]);

  useEffect(() => {
    const timer = window.setTimeout(() => { void loadData(); }, 0);
    return () => window.clearTimeout(timer);
  }, [loadData]);

  const handleCreate = () => {
    setEditingRoute(null);
    setForm(emptyForm);
    setDialogOpen(true);
  };

  const handleEdit = (route: RouteItem) => {
    const hops = [...(route.hops || [])].sort((a, b) => a.hopOrder - b.hopOrder);
    setEditingRoute(route);
    setForm({
      name: route.name || '',
      nodeIds: hops.length >= 2 ? hops.map(hop => String(hop.nodeId)) : ['', ''],
      protocol: route.protocol || 'tls',
      tcpListenAddr: route.tcpListenAddr || '::',
      udpListenAddr: route.udpListenAddr || '::',
      interfaceName: route.interfaceName || '',
      status: String(route.status ?? 1),
    });
    setDialogOpen(true);
  };

  const updateHop = (index: number, value: string) => {
    setForm(prev => ({
      ...prev,
      nodeIds: prev.nodeIds.map((nodeId, i) => (i === index ? value : nodeId)),
    }));
  };

  const addHop = () => {
    setForm(prev => ({ ...prev, nodeIds: [...prev.nodeIds, ''] }));
  };

  const removeHop = (index: number) => {
    setForm(prev => ({ ...prev, nodeIds: prev.nodeIds.filter((_, i) => i !== index) }));
  };

  const handleSubmit = async () => {
    const nodeIds = form.nodeIds.map(id => parseInt(id, 10)).filter(Boolean);
    if (!form.name.trim() || nodeIds.length < 2 || nodeIds.length !== form.nodeIds.length) {
      toast.error(t('common.fillRequired'));
      return;
    }
    if (new Set(nodeIds).size !== nodeIds.length) {
      toast.error(t('route.uniqueNodes'));
      return;
    }

    const data = {
      name: form.name.trim(),
      nodeIds,
      protocol: form.protocol,
      tcpListenAddr: form.tcpListenAddr.trim() || '::',
      udpListenAddr: form.udpListenAddr.trim() || '::',
      interfaceName: form.interfaceName.trim(),
      status: parseInt(form.status, 10),
    };

    const res = editingRoute
      ? await updateRoute({ ...data, id: editingRoute.id })
      : await createRoute(data);

    if (res.code === 0) {
      toast.success(editingRoute ? t('common.updateSuccess') : t('common.createSuccess'));
      setDialogOpen(false);
      await loadData();
    } else {
      toast.error(res.msg);
    }
  };

  const handleDelete = async (route: RouteItem) => {
    if (!confirm(t('route.confirmDelete', { name: route.name }))) return;
    const res = await deleteRoute(route.id);
    if (res.code === 0) {
      toast.success(t('common.deleteSuccess'));
      await loadData();
    } else {
      toast.error(res.msg);
    }
  };

  const nodeLabel = (nodeId: number, fallback?: string) => {
    const node = nodeById.get(nodeId);
    return node?.name || fallback || `${t('route.nodePrefix')} ${nodeId}`;
  };

  const renderPath = (route: RouteItem) => {
    const hops = [...(route.hops || [])].sort((a, b) => a.hopOrder - b.hopOrder);
    if (hops.length === 0) return <span className="text-muted-foreground">-</span>;
    return (
      <div className="flex flex-wrap items-center gap-1.5">
        {hops.map((hop, index) => (
          <div key={`${route.id}-${hop.hopOrder}-${hop.nodeId}`} className="flex items-center gap-1.5">
            {index > 0 && <span className="text-muted-foreground">-&gt;</span>}
            <Badge variant="secondary" className="rounded-md">
              {index + 1}. {nodeLabel(hop.nodeId, hop.nodeName)}
            </Badge>
          </div>
        ))}
      </div>
    );
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
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold">{t('route.title')}</h2>
        <Button onClick={handleCreate}><Plus className="mr-2 h-4 w-4" />{t('route.createRoute')}</Button>
      </div>

      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('route.name')}</TableHead>
                <TableHead>{t('route.path')}</TableHead>
                <TableHead>{t('route.protocol')}</TableHead>
                <TableHead>{t('route.listen')}</TableHead>
                <TableHead>{t('route.latency')}</TableHead>
                <TableHead>{t('route.interface')}</TableHead>
                <TableHead>{t('route.status')}</TableHead>
                <TableHead className="text-right">{t('route.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                <TableRow><TableCell colSpan={8} className="py-8 text-center text-muted-foreground">{t('route.loading')}</TableCell></TableRow>
              ) : routes.length === 0 ? (
                <TableRow><TableCell colSpan={8} className="py-8 text-center text-muted-foreground">{t('common.noData')}</TableCell></TableRow>
              ) : routes.map(route => (
                <TableRow key={route.id}>
                  <TableCell className="font-medium">{route.name}</TableCell>
                  <TableCell>{renderPath(route)}</TableCell>
                  <TableCell>{route.protocol || 'tls'}</TableCell>
                  <TableCell>
                    <div className="space-y-0.5 text-sm">
                      <div>TCP {route.tcpListenAddr || '::'}</div>
                      <div className="text-muted-foreground">UDP {route.udpListenAddr || '::'}</div>
                    </div>
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-sm">
                    {typeof route.latency === 'number'
                      ? `${route.latency.toFixed(1)} ms`
                      : <span className="text-muted-foreground">-</span>}
                  </TableCell>
                  <TableCell>{route.interfaceName || '-'}</TableCell>
                  <TableCell>
                    <Badge variant={(route.status ?? 1) === 1 ? 'default' : 'secondary'} className="rounded-md">
                      {(route.status ?? 1) === 1 ? t('common.enabled') : t('common.disabled')}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-2">
                      <Button variant="outline" size="icon" onClick={() => handleEdit(route)}>
                        <Edit2 className="h-4 w-4" />
                      </Button>
                      <Button variant="destructive" size="icon" onClick={() => handleDelete(route)}>
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{editingRoute ? t('route.editRoute') : t('route.createRoute')}</DialogTitle>
          </DialogHeader>

          <div className="space-y-4">
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label>{t('route.name')}</Label>
                <Input value={form.name} onChange={e => setForm(prev => ({ ...prev, name: e.target.value }))} />
              </div>
              <div className="space-y-2">
                <Label>{t('route.protocol')}</Label>
                <Select value={form.protocol} onValueChange={value => setForm(prev => ({ ...prev, protocol: value }))}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="tls">TLS</SelectItem>
                    <SelectItem value="tcp">TCP</SelectItem>
                    <SelectItem value="ws">WS</SelectItem>
                    <SelectItem value="wss">WSS</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="grid gap-4 md:grid-cols-3">
              <div className="space-y-2">
                <Label>{t('route.tcpListen')}</Label>
                <Input value={form.tcpListenAddr} onChange={e => setForm(prev => ({ ...prev, tcpListenAddr: e.target.value }))} placeholder="::" />
              </div>
              <div className="space-y-2">
                <Label>{t('route.udpListen')}</Label>
                <Input value={form.udpListenAddr} onChange={e => setForm(prev => ({ ...prev, udpListenAddr: e.target.value }))} placeholder="::" />
              </div>
              <div className="space-y-2">
                <Label>{t('route.interface')}</Label>
                <Input value={form.interfaceName} onChange={e => setForm(prev => ({ ...prev, interfaceName: e.target.value }))} placeholder="auto" />
              </div>
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>{t('route.hops')}</Label>
                <Button type="button" variant="outline" size="sm" onClick={addHop}><Plus className="mr-2 h-4 w-4" />{t('route.addHop')}</Button>
              </div>
              <div className="space-y-2">
                {form.nodeIds.map((nodeId, index) => {
                  const selected = new Set(form.nodeIds.filter((_, i) => i !== index));
                  return (
                    <div key={index} className="grid grid-cols-[48px_1fr_40px] items-center gap-2">
                      <Badge variant="outline" className="justify-center rounded-md">{index + 1}</Badge>
                      <Select value={nodeId} onValueChange={value => updateHop(index, value)}>
                        <SelectTrigger><SelectValue placeholder={t('route.selectNode')} /></SelectTrigger>
                        <SelectContent>
                          {nodes.map(node => (
                            <SelectItem key={node.id} value={String(node.id)} disabled={selected.has(String(node.id))}>
                              {node.name}{node.serverIp ? ` (${node.serverIp})` : ''}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <Button type="button" variant="outline" size="icon" onClick={() => removeHop(index)} disabled={form.nodeIds.length <= 2}>
                        <X className="h-4 w-4" />
                      </Button>
                    </div>
                  );
                })}
              </div>
            </div>

            {editingRoute && (
              <div className="space-y-2">
                <Label>{t('route.status')}</Label>
                <Select value={form.status} onValueChange={value => setForm(prev => ({ ...prev, status: value }))}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="1">{t('common.enabled')}</SelectItem>
                    <SelectItem value="0">{t('common.disabled')}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>{t('common.cancel')}</Button>
            <Button onClick={handleSubmit}>{editingRoute ? t('common.update') : t('common.create')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
