'use client';

import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Plus, Trash2, Edit2, Copy, Eye, EyeOff, RefreshCw, Check, Zap } from 'lucide-react';
import { toast } from 'sonner';
import { getNodeList, createNode, updateNode, deleteNode, getNodeLiteInstallCommand, reconcileNode, setNodeProtocol } from '@/lib/api/node';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Switch } from '@/components/ui/switch';
import { useAuth } from '@/lib/hooks/use-auth';
import { useTranslation } from '@/lib/i18n';

function normalizeVersionLabel(version: unknown): string {
  if (typeof version !== 'string') return '';
  return version.trim().replace(/^v(?=\d)/i, '');
}

export default function NodePage() {
  const { isAdmin } = useAuth();
  const { t } = useTranslation();
  const [nodes, setNodes] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingNode, setEditingNode] = useState<any>(null);
  const [form, setForm] = useState({ name: '', entryIps: '', serverIp: '', portSta: '', portEnd: '', secret: '', groupName: '' });
  const [commandDialog, setCommandDialog] = useState(false);
  const [commandContent, setCommandContent] = useState('');
  const [commandTitle, setCommandTitle] = useState('');
  const [commandIPv6, setCommandIPv6] = useState(false);
  const [showSecret, setShowSecret] = useState(false);
  const [protocolForm, setProtocolForm] = useState({ http: 0, tls: 0, socks: 0 });
  const [protocolSaving, setProtocolSaving] = useState(false);

  const initialLoad = useRef(true);

  // WebSocket for uptime updates
  useEffect(() => {
    const token = localStorage.getItem('token');
    if (!token) return;

    const wsBase = process.env.NEXT_PUBLIC_API_BASE || window.location.origin;
    const wsUrl = wsBase.replace(/^http/, 'ws') + '/system-info?type=0';

    let ws: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout>;

    const connect = () => {
      ws = new WebSocket(wsUrl, [token]);

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data);
          if (msg.type === 'info' && msg.data) {
            const sysData = typeof msg.data === 'string' ? JSON.parse(msg.data) : msg.data;
            const nodeId = parseInt(msg.id, 10);

            if (sysData.uptime !== undefined) {
              setNodes(prev => prev.map(n =>
                n.id === nodeId
                  ? { ...n, uptime: sysData.uptime }
                  : n
              ));
            }
          }
        } catch (e) {
          console.warn('Failed to parse WebSocket message:', e);
        }
      };

      ws.onclose = () => {
        reconnectTimer = setTimeout(connect, 5000);
      };

      ws.onerror = () => {
        ws?.close();
      };
    };

    connect();

    return () => {
      clearTimeout(reconnectTimer);
      ws?.close();
    };
  }, []);

  // Sort nodes by group for separator rows
  const sortedNodes = useMemo(() => {
    return [...nodes].sort((a, b) => {
      const ga = a.groupName || '';
      const gb = b.groupName || '';
      if (ga === '' && gb !== '') return 1;
      if (ga !== '' && gb === '') return -1;
      if (ga !== gb) return ga.localeCompare(gb);
      return 0;
    });
  }, [nodes]);

  const hasGroups = useMemo(() => nodes.some(n => n.groupName && n.groupName.length > 0), [nodes]);

  const loadData = useCallback(async () => {
    if (initialLoad.current) setLoading(true);
    const res = await getNodeList();
    if (res.code === 0) setNodes(res.data || []);
    setLoading(false);
    initialLoad.current = false;
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  // Auto-refresh every 30 seconds
  useEffect(() => {
    const timer = setInterval(() => { loadData(); }, 30000);
    return () => clearInterval(timer);
  }, [loadData]);

  const handleCreate = () => {
    setEditingNode(null);
    setForm({ name: '', entryIps: '', serverIp: '', portSta: '10000', portEnd: '60000', secret: '', groupName: '' });
    setShowSecret(false);
    setDialogOpen(true);
  };

  const handleEdit = (node: any) => {
    setEditingNode(node);
    setForm({
      name: node.name || '',
      entryIps: node.entryIps?.includes(',') ? node.entryIps.split(',').join('\n') : (node.entryIps || ''),
      serverIp: node.serverIp || '',
      portSta: node.portSta?.toString() || '',
      portEnd: node.portEnd?.toString() || '',
      secret: node.secret || '',
      groupName: node.groupName || '',
    });
    setShowSecret(false);
    setProtocolForm({ http: node.http || 0, tls: node.tls || 0, socks: node.socks || 0 });
    setDialogOpen(true);
  };

  const handleProtocolToggle = async (field: 'http' | 'tls' | 'socks', value: boolean) => {
    if (!editingNode) return;
    const oldForm = { ...protocolForm };
    const newForm = { ...protocolForm, [field]: value ? 1 : 0 };
    setProtocolForm(newForm);
    setProtocolSaving(true);
    try {
      const res = await setNodeProtocol({ id: editingNode.id, ...newForm });
      if (res.code === 0) {
        toast.success(t('node.protocolUpdateSuccess'));
        setNodes(prev => prev.map(n =>
          n.id === editingNode.id ? { ...n, ...newForm } : n
        ));
      } else {
        toast.error(res.msg || t('node.protocolUpdateFailed'));
        setProtocolForm(oldForm);
      }
    } catch {
      toast.error(t('node.protocolUpdateFailed'));
      setProtocolForm(oldForm);
    } finally {
      setProtocolSaving(false);
    }
  };

  const handleSubmit = async () => {
    if (!form.name || !form.serverIp) {
      toast.error(t('common.fillRequired'));
      return;
    }
    const data: any = {
      name: form.name,
      entryIps: form.entryIps.split('\n').map(s => s.trim()).filter(Boolean).join(','),
      serverIp: form.serverIp,
      secret: form.secret || undefined,
      groupName: form.groupName,
    };
    if (form.portSta) data.portSta = parseInt(form.portSta);
    if (form.portEnd) data.portEnd = parseInt(form.portEnd);

    let res;
    if (editingNode) {
      res = await updateNode({ ...data, id: editingNode.id });
    } else {
      res = await createNode(data);
    }

    if (res.code === 0) {
      toast.success(editingNode ? t('common.updateSuccess') : t('common.createSuccess'));
      setDialogOpen(false);
      loadData();
    } else {
      toast.error(res.msg);
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm(t('node.confirmDeleteNode'))) return;
    const res = await deleteNode(id);
    if (res.code === 0) { toast.success(t('common.deleteSuccess')); loadData(); }
    else toast.error(res.msg);
  };

  const handleLiteInstallCommand = async (node: any) => {
    const res = await getNodeLiteInstallCommand(node.id);
    if (res.code === 0) {
      setCommandTitle(t('node.liteInstallCommand'));
      setCommandContent(res.data || '');
      setCommandNode(node);
      setCommandDialog(true);
    } else {
      toast.error(res.msg);
    }
  };

  const [commandNode, setCommandNode] = useState<any>(null);
  const [reconcilingId, setReconcilingId] = useState<number | null>(null);

  const handleReconcile = async (id: number) => {
    setReconcilingId(id);
    try {
      const res = await reconcileNode(id);
      if (res.code === 0) {
        const d = res.data;
        toast.success(t('node.syncResult', { limiters: d.limiters, forwards: d.forwards, inbounds: d.inbounds, certs: d.certs, duration: d.duration }));
        if (d.errors && d.errors.length > 0) {
          toast.warning(t('node.syncErrors', { count: d.errors.length, first: d.errors[0] }));
        }
      } else {
        toast.error(res.msg);
      }
    } finally {
      setReconcilingId(null);
    }
  };

  const [copied, setCopied] = useState(false);
  const copyToClipboard = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      toast.success(t('common.copySuccess'));
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback for insecure contexts
      const textarea = document.createElement('textarea');
      textarea.value = text;
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand('copy');
      document.body.removeChild(textarea);
      setCopied(true);
      toast.success(t('common.copySuccess'));
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const formatUptime = (seconds: number) => {
    if (!seconds) return '-';
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    if (days > 0) return `${days}${t('node.days')}${hours}${t('node.hours')}${mins}${t('node.minutes')}`;
    if (hours > 0) return `${hours}${t('node.hours')}${mins}${t('node.minutes')}`;
    return `${mins}${t('node.minutes')}`;
  };


  const renderSingBoxAuditStatus = (node: any) => {
    if (node.status !== 1) return '-';
    const audit = node.singBoxAudit;
    if (!audit || !audit.tailerRunning) {
      return <Badge variant="secondary" className="text-xs">{t('node.singBoxAuditUpdateNeeded')}</Badge>;
    }
    if (!audit.logReadable) {
      return <Badge variant="outline" className="text-xs border-orange-400 text-orange-600">{t('node.singBoxAuditNoLog')}</Badge>;
    }
    return <Badge variant="default" className="text-xs">{t('node.singBoxAuditActive')}</Badge>;
  };
  if (!isAdmin) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-muted-foreground">无权限访问</p>
      </div>
    );
  }

  const renderTableRows = () => {
    const rows: React.ReactNode[] = [];
    let lastGroup: string | null = null;
    const totalCols = 12;
    for (const n of sortedNodes) {
      const group = n.groupName || '';
      if (hasGroups && group !== lastGroup) {
        rows.push(
          <TableRow key={`group-${group}`} className="bg-muted/50 hover:bg-muted/50">
            <TableCell colSpan={totalCols} className="py-1.5 px-4 text-xs font-semibold text-muted-foreground uppercase tracking-wider">
              {group || t('monitor.ungrouped')}
            </TableCell>
          </TableRow>
        );
        lastGroup = group;
      }
      const isOnline = n.status === 1;
      const nodeVersion = normalizeVersionLabel(n.version);
      rows.push(
        <TableRow key={n.id}>
          <TableCell className="font-medium text-sm">{n.name}</TableCell>
          <TableCell className="text-xs">
            <div title={t('node.disguiseName')}>{n.disguiseName || '-'}</div>
            <div className="text-muted-foreground" title={t('node.xrayDisguiseName')}>{n.xrayDisguiseName || '-'}</div>
          </TableCell>
          <TableCell className="text-sm whitespace-pre-line">{n.entryIps ? n.entryIps.split(',').join('\n') : (n.ip || '-')}</TableCell>
          <TableCell className="text-sm">
            <div>{n.serverIp}</div>
            {isOnline && n.panelAddr && (
              <div className="text-xs text-muted-foreground truncate max-w-[180px]" title={n.panelAddr}>
                {t('monitor.panelAddr')}: {n.panelAddr}
              </div>
            )}
          </TableCell>
          <TableCell className="text-sm">{n.portSta} - {n.portEnd}</TableCell>
          <TableCell>
            <div className="flex items-center gap-1">
              <Badge variant={isOnline ? 'default' : 'destructive'} className="text-xs">
                {isOnline ? t('common.online') : t('common.offline')}
              </Badge>
              {isOnline && n.runtime && (
                <Badge variant={n.runtime === 'docker' ? 'outline' : 'secondary'} className="text-xs">
                  {n.runtime === 'docker' ? 'D' : 'H'}
                </Badge>
              )}
            </div>
          </TableCell>
          <TableCell className="text-sm">
            {isOnline ? <Badge variant="default" className="text-xs">{t('monitor.running')}</Badge> : '-'}
          </TableCell>
          <TableCell className="text-sm">
            {isOnline ? (
              n.vRunning ? (
                <Badge variant="default" className="text-xs">{t('monitor.running')}</Badge>
              ) : (
                <Badge variant="secondary" className="text-xs">{t('monitor.notRunning')}</Badge>
              )
            ) : '-'}
          </TableCell>
          <TableCell className="text-sm">

            {renderSingBoxAuditStatus(n)}

          </TableCell>

          <TableCell className="text-sm">
            <span>{nodeVersion || '-'}</span>
          </TableCell>
          <TableCell className="text-sm">{formatUptime(n.uptime)}</TableCell>
          <TableCell>
            <div className="flex gap-1">
              <Button variant="ghost" size="icon" onClick={() => handleEdit(n)} title="编辑">
                <Edit2 className="h-4 w-4" />
              </Button>
              <Button variant="ghost" size="icon" onClick={() => handleReconcile(n.id)} disabled={reconcilingId === n.id} title={t('node.syncConfig')}>
                <RefreshCw className={`h-4 w-4 ${reconcilingId === n.id ? 'animate-spin' : ''}`} />
              </Button>
              <Button variant="ghost" size="icon" onClick={() => handleLiteInstallCommand(n)} title={t('node.liteInstallCommand')}>
                <Zap className="h-4 w-4" />
              </Button>
              <Button variant="ghost" size="icon" onClick={() => handleDelete(n.id)} className="text-destructive" title="删除">
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          </TableCell>
        </TableRow>
      );
    }
    return rows;
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold">{t('node.title')}</h2>
        <Button onClick={handleCreate}><Plus className="mr-2 h-4 w-4" />{t('node.createNode')}</Button>
      </div>

      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('node.name')}</TableHead>
                <TableHead>{t('node.disguiseName')}</TableHead>
                <TableHead>{t('node.entryIp')}</TableHead>
                <TableHead>{t('node.serverIp')}</TableHead>
                <TableHead>{t('node.portRange')}</TableHead>
                <TableHead>{t('node.status')}</TableHead>
                <TableHead>GOST</TableHead>
                <TableHead>Xray</TableHead>
                <TableHead>{t('node.singBoxAudit')}</TableHead>
                <TableHead>{t('node.version')}</TableHead>
                <TableHead>{t('node.uptime')}</TableHead>
                <TableHead>{t('node.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                <TableRow><TableCell colSpan={12} className="text-center py-8">{t('common.loading')}</TableCell></TableRow>
              ) : nodes.length === 0 ? (
                <TableRow><TableCell colSpan={12} className="text-center py-8 text-muted-foreground">{t('common.noData')}</TableCell></TableRow>
              ) : (
                renderTableRows()
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {/* Create/Edit Node Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingNode ? t('node.editNode') : t('node.createNodeTitle')}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t('node.name')}</Label>
              <Input value={form.name} onChange={e => setForm(p => ({ ...p, name: e.target.value }))} placeholder={t('node.nodeName')} />
            </div>
            <div className="space-y-2">
              <Label>{t('node.groupName')}</Label>
              <Input value={form.groupName} onChange={e => setForm(p => ({ ...p, groupName: e.target.value }))} placeholder={t('node.groupNamePlaceholder')} />
            </div>
            <div className="space-y-2">
              <Label>{t('node.entryIpList')}</Label>
              <Textarea
                value={form.entryIps}
                onChange={e => setForm(p => ({ ...p, entryIps: e.target.value }))}
                placeholder={"每行一个IP，例如:\n1.2.3.4\n5.6.7.8\n2001:db8::1"}
                rows={3}
                className="font-mono text-sm"
              />
              <p className="text-xs text-muted-foreground">{t('node.entryIpDesc')}</p>
            </div>
            <div className="space-y-2">
              <Label>{t('node.serverIpLabel')}</Label>
              <Input value={form.serverIp} onChange={e => setForm(p => ({ ...p, serverIp: e.target.value }))} placeholder={t('node.serverIpPlaceholder')} />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>{t('node.startPort')}</Label>
                <Input value={form.portSta} onChange={e => setForm(p => ({ ...p, portSta: e.target.value }))} placeholder="10000" autoComplete="off" />
              </div>
              <div className="space-y-2">
                <Label>{t('node.endPort')}</Label>
                <Input value={form.portEnd} onChange={e => setForm(p => ({ ...p, portEnd: e.target.value }))} placeholder="60000" autoComplete="off" />
              </div>
            </div>
            <div className="space-y-2">
              <Label>{t('node.commSecret')}</Label>
              <div className="relative">
                <Input
                  type={showSecret ? 'text' : 'password'}
                  value={form.secret}
                  onChange={e => setForm(p => ({ ...p, secret: e.target.value }))}
                  placeholder={t('node.secretAutoGenerate')}
                  readOnly={!!editingNode}
                  className={editingNode ? 'bg-muted' : ''}
                  autoComplete="off"
                />
                <Button
                  variant="ghost"
                  size="icon"
                  className="absolute right-0 top-0"
                  onClick={() => setShowSecret(!showSecret)}
                >
                  {showSecret ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </Button>
              </div>
              {editingNode && <p className="text-xs text-muted-foreground">{t('node.secretReadonly')}</p>}
            </div>
            {editingNode && (
              <div className="space-y-2">
                <Label>{t('node.protocolBlock')}</Label>
                {editingNode.status !== 1 ? (
                  <p className="text-xs text-muted-foreground">{t('node.nodeOfflineCannotSet')}</p>
                ) : (
                  <>
                    <div className="flex items-center gap-6">
                      <label className="flex items-center gap-2 text-sm">
                        <Switch
                          size="sm"
                          checked={protocolForm.http === 1}
                          onCheckedChange={(v) => handleProtocolToggle('http', v)}
                          disabled={protocolSaving}
                        />
                        HTTP
                      </label>
                      <label className="flex items-center gap-2 text-sm">
                        <Switch
                          size="sm"
                          checked={protocolForm.tls === 1}
                          onCheckedChange={(v) => handleProtocolToggle('tls', v)}
                          disabled={protocolSaving}
                        />
                        TLS
                      </label>
                      <label className="flex items-center gap-2 text-sm">
                        <Switch
                          size="sm"
                          checked={protocolForm.socks === 1}
                          onCheckedChange={(v) => handleProtocolToggle('socks', v)}
                          disabled={protocolSaving}
                        />
                        SOCKS
                      </label>
                    </div>
                    <p className="text-xs text-muted-foreground">{t('node.protocolBlockDesc')}</p>
                  </>
                )}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>{t('common.cancel')}</Button>
            <Button onClick={handleSubmit}>{editingNode ? t('common.update') : t('common.create')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Command Dialog */}
      <Dialog open={commandDialog} onOpenChange={setCommandDialog}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{commandTitle}</DialogTitle>
          </DialogHeader>
          <Tabs value={commandIPv6 ? 'ipv6' : 'standard'} onValueChange={(v) => setCommandIPv6(v === 'ipv6')}>
            <TabsList className="mb-2">
              <TabsTrigger value="standard">{t('node.standardNetwork')}</TabsTrigger>
              <TabsTrigger value="ipv6">{t('node.ipv6Network')}</TabsTrigger>
            </TabsList>
          </Tabs>
          <div className="space-y-3">
            <pre className="bg-muted p-4 rounded-md text-sm overflow-x-auto whitespace-pre-wrap break-all">
              {commandIPv6
                ? commandContent
                    .replace('curl -fsSL', 'curl -6fsSL')
                    .replace('| bash', '| bash -s -- 6')
                    .replace('| sh', '| sh -s -- 6')
                : commandContent}
            </pre>
            <div className="flex justify-end">
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  const cmd = commandIPv6
                    ? commandContent
                        .replace('curl -fsSL', 'curl -6fsSL')
                        .replace('| bash', '| bash -s -- 6')
                        .replace('| sh', '| sh -s -- 6')
                    : commandContent;
                  copyToClipboard(cmd);
                }}
              >
                {copied ? <Check className="mr-2 h-3 w-3" /> : <Copy className="mr-2 h-3 w-3" />}
                {copied ? t('common.copied') : t('common.copy')}
              </Button>
            </div>
            <div className="border-t pt-3">
              <p className="text-sm font-medium mb-2 text-muted-foreground">{t('node.uninstallCommand')}</p>
              <pre className="bg-muted p-4 rounded-md text-sm overflow-x-auto whitespace-pre-wrap break-all">
                {`sh /etc/${commandNode?.disguiseName || 'gost-node-lite'}/uninstall.sh`}
              </pre>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCommandDialog(false)}>{t('common.close')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
