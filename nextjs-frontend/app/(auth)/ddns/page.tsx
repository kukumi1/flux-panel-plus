'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Plus, Edit2, Trash2, RefreshCw } from 'lucide-react';
import { toast } from 'sonner';
import {
  createDdnsDomain, createDdnsProvider, deleteDdnsDomain, deleteDdnsProvider,
  getDdnsDomainList, getDdnsProviderList, getDdnsProviderTypes, resolveDdnsDomain,
  updateDdnsDomain, updateDdnsProvider,
  type DdnsDomain, type DdnsProvider,
} from '@/lib/api/ddns';
import { getGroupList, type NodeGroup } from '@/lib/api/group';
import { useAuth } from '@/lib/hooks/use-auth';
import { useTranslation } from '@/lib/i18n';

type ProviderForm = {
  name: string;
  type: string;
  apiToken: string;
  url: string;
  method: string;
  body: string;
};

type DomainForm = {
  providerId: string;
  groupId: string;
  domain: string;
  recordName: string;
  recordType: string;
  autoResolve: boolean;
};

const emptyProviderForm: ProviderForm = {
  name: '',
  type: 'cloudflare',
  apiToken: '',
  url: '',
  method: 'POST',
  body: '',
};

const emptyDomainForm: DomainForm = {
  providerId: '',
  groupId: '',
  domain: '',
  recordName: '',
  recordType: 'A',
  autoResolve: true,
};

function formatTime(ts: number) {
  if (!ts) return '-';
  return new Date(ts).toLocaleString();
}

export default function DdnsPage() {
  const { isAdmin } = useAuth();
  const { t } = useTranslation();
  const [providers, setProviders] = useState<DdnsProvider[]>([]);
  const [domains, setDomains] = useState<DdnsDomain[]>([]);
  const [groups, setGroups] = useState<NodeGroup[]>([]);
  const [providerTypes, setProviderTypes] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);

  const [providerDialogOpen, setProviderDialogOpen] = useState(false);
  const [editingProvider, setEditingProvider] = useState<DdnsProvider | null>(null);
  const [providerForm, setProviderForm] = useState<ProviderForm>(emptyProviderForm);

  const [domainDialogOpen, setDomainDialogOpen] = useState(false);
  const [editingDomain, setEditingDomain] = useState<DdnsDomain | null>(null);
  const [domainForm, setDomainForm] = useState<DomainForm>(emptyDomainForm);

  const providerById = useMemo(() => {
    const map = new Map<number, DdnsProvider>();
    for (const p of providers) map.set(p.id, p);
    return map;
  }, [providers]);

  const groupById = useMemo(() => {
    const map = new Map<number, NodeGroup>();
    for (const g of groups) map.set(g.id, g);
    return map;
  }, [groups]);

  const loadData = useCallback(async () => {
    try {
      const [providerRes, domainRes, groupRes, typeRes] = await Promise.all([
        getDdnsProviderList(), getDdnsDomainList(), getGroupList(), getDdnsProviderTypes(),
      ]);
      if (providerRes.code === 0) setProviders(providerRes.data || []);
      if (domainRes.code === 0) setDomains(domainRes.data || []);
      if (groupRes.code === 0) setGroups(groupRes.data || []);
      if (typeRes.code === 0) setProviderTypes(typeRes.data || []);
    } catch {
      toast.error(t('common.loadFailed'));
    }
    setLoading(false);
  }, [t]);

  useEffect(() => {
    const timer = window.setTimeout(() => { void loadData(); }, 0);
    return () => window.clearTimeout(timer);
  }, [loadData]);

  const handleCreateProvider = () => {
    setEditingProvider(null);
    setProviderForm({ ...emptyProviderForm, type: providerTypes[0] || 'cloudflare' });
    setProviderDialogOpen(true);
  };

  const handleEditProvider = (provider: DdnsProvider) => {
    setEditingProvider(provider);
    setProviderForm({ ...emptyProviderForm, name: provider.name, type: provider.type });
    setProviderDialogOpen(true);
  };

  const buildCredential = (form: ProviderForm): unknown | null => {
    if (form.type === 'cloudflare') {
      if (!form.apiToken.trim()) return null;
      return { apiToken: form.apiToken.trim() };
    }
    if (form.type === 'webhook') {
      if (!form.url.trim()) return null;
      return { url: form.url.trim(), method: form.method, body: form.body };
    }
    return null;
  };

  const handleSubmitProvider = async () => {
    if (!providerForm.name.trim()) {
      toast.error(t('common.fillRequired'));
      return;
    }
    const credential = buildCredential(providerForm);

    if (editingProvider) {
      const res = await updateDdnsProvider({
        id: editingProvider.id,
        name: providerForm.name.trim(),
        ...(credential ? { credential } : {}),
      });
      if (res.code === 0) {
        toast.success(t('common.updateSuccess'));
        setProviderDialogOpen(false);
        await loadData();
      } else {
        toast.error(res.msg);
      }
      return;
    }

    if (!credential) {
      toast.error(t('ddns.credentialRequired'));
      return;
    }
    const res = await createDdnsProvider({ name: providerForm.name.trim(), type: providerForm.type, credential });
    if (res.code === 0) {
      toast.success(t('common.createSuccess'));
      setProviderDialogOpen(false);
      await loadData();
    } else {
      toast.error(res.msg);
    }
  };

  const handleDeleteProvider = async (provider: DdnsProvider) => {
    if (!confirm(t('ddns.confirmDeleteProvider', { name: provider.name }))) return;
    const res = await deleteDdnsProvider(provider.id);
    if (res.code === 0) {
      toast.success(t('common.deleteSuccess'));
      await loadData();
    } else {
      toast.error(res.msg);
    }
  };

  const handleCreateDomain = () => {
    setEditingDomain(null);
    setDomainForm({
      ...emptyDomainForm,
      providerId: providers[0] ? String(providers[0].id) : '',
      groupId: groups[0] ? String(groups[0].id) : '',
    });
    setDomainDialogOpen(true);
  };

  const handleEditDomain = (domain: DdnsDomain) => {
    setEditingDomain(domain);
    setDomainForm({
      providerId: String(domain.providerId),
      groupId: String(domain.groupId),
      domain: domain.domain,
      recordName: domain.recordName || '',
      recordType: domain.recordType || 'A',
      autoResolve: domain.autoResolve === 1,
    });
    setDomainDialogOpen(true);
  };

  const handleSubmitDomain = async () => {
    const providerId = parseInt(domainForm.providerId, 10);
    const groupId = parseInt(domainForm.groupId, 10);
    if (!domainForm.domain.trim() || !providerId || !groupId) {
      toast.error(t('common.fillRequired'));
      return;
    }

    const res = editingDomain
      ? await updateDdnsDomain({
          id: editingDomain.id,
          providerId,
          domain: domainForm.domain.trim(),
          recordName: domainForm.recordName.trim(),
          recordType: domainForm.recordType,
          autoResolve: domainForm.autoResolve ? 1 : 0,
        })
      : await createDdnsDomain({
          providerId,
          groupId,
          domain: domainForm.domain.trim(),
          recordName: domainForm.recordName.trim(),
          recordType: domainForm.recordType,
          autoResolve: domainForm.autoResolve ? 1 : 0,
        });

    if (res.code === 0) {
      toast.success(editingDomain ? t('common.updateSuccess') : t('common.createSuccess'));
      setDomainDialogOpen(false);
      await loadData();
    } else {
      toast.error(res.msg);
    }
  };

  const handleDeleteDomain = async (domain: DdnsDomain) => {
    if (!confirm(t('ddns.confirmDeleteDomain', { name: domain.domain }))) return;
    const res = await deleteDdnsDomain(domain.id);
    if (res.code === 0) {
      toast.success(t('common.deleteSuccess'));
      await loadData();
    } else {
      toast.error(res.msg);
    }
  };

  const handleResolve = async (domain: DdnsDomain) => {
    const res = await resolveDdnsDomain(domain.id);
    if (res.code === 0) {
      toast.success(res.msg || t('ddns.resolveSuccess'));
      await loadData();
    } else {
      toast.error(res.msg);
    }
  };

  const fqdnLabel = (domain: DdnsDomain) =>
    (!domain.recordName || domain.recordName === '@') ? domain.domain : `${domain.recordName}.${domain.domain}`;

  if (!isAdmin) {
    return (
      <div className="flex h-64 items-center justify-center">
        <p className="text-muted-foreground">{t('common.noPermission')}</p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-2xl font-bold">{t('ddns.title')}</h2>
        <p className="text-sm text-muted-foreground">{t('ddns.description')}</p>
      </div>

      <Tabs defaultValue="domains" className="w-full">
        <TabsList>
          <TabsTrigger value="domains">{t('ddns.domainsTab')}</TabsTrigger>
          <TabsTrigger value="providers">{t('ddns.providersTab')}</TabsTrigger>
        </TabsList>

        <TabsContent value="domains" className="space-y-4">
          <div className="flex justify-end">
            <Button onClick={handleCreateDomain} disabled={providers.length === 0 || groups.length === 0}>
              <Plus className="mr-2 h-4 w-4" />{t('ddns.createDomain')}
            </Button>
          </div>
          {(providers.length === 0 || groups.length === 0) && (
            <p className="text-sm text-muted-foreground">{t('ddns.domainPrereq')}</p>
          )}
          <Card>
            <CardContent className="p-0">
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('ddns.domain')}</TableHead>
                      <TableHead>{t('ddns.provider')}</TableHead>
                      <TableHead>{t('ddns.group')}</TableHead>
                      <TableHead>{t('ddns.recordType')}</TableHead>
                      <TableHead>{t('ddns.autoResolve')}</TableHead>
                      <TableHead>{t('ddns.currentRecord')}</TableHead>
                      <TableHead className="text-right">{t('ddns.actions')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {loading ? (
                      <TableRow><TableCell colSpan={7} className="py-8 text-center text-muted-foreground">{t('common.loading')}</TableCell></TableRow>
                    ) : domains.length === 0 ? (
                      <TableRow><TableCell colSpan={7} className="py-8 text-center text-muted-foreground">{t('common.noData')}</TableCell></TableRow>
                    ) : domains.map(domain => (
                      <TableRow key={domain.id}>
                        <TableCell className="font-medium">{fqdnLabel(domain)}</TableCell>
                        <TableCell>{providerById.get(domain.providerId)?.name || `#${domain.providerId}`}</TableCell>
                        <TableCell>{groupById.get(domain.groupId)?.name || `#${domain.groupId}`}</TableCell>
                        <TableCell><Badge variant="outline" className="rounded-md">{domain.recordType}</Badge></TableCell>
                        <TableCell>
                          <Badge variant={domain.autoResolve === 1 ? 'default' : 'secondary'} className="rounded-md">
                            {domain.autoResolve === 1 ? t('common.enabled') : t('common.disabled')}
                          </Badge>
                        </TableCell>
                        <TableCell className="font-mono text-sm">{domain.currentRecord || '-'}</TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button variant="outline" size="icon" onClick={() => handleResolve(domain)} title={t('ddns.resolveNow')}>
                              <RefreshCw className="h-4 w-4" />
                            </Button>
                            <Button variant="outline" size="icon" onClick={() => handleEditDomain(domain)}>
                              <Edit2 className="h-4 w-4" />
                            </Button>
                            <Button variant="destructive" size="icon" onClick={() => handleDeleteDomain(domain)}>
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="providers" className="space-y-4">
          <div className="flex justify-end">
            <Button onClick={handleCreateProvider}><Plus className="mr-2 h-4 w-4" />{t('ddns.createProvider')}</Button>
          </div>
          <Card>
            <CardContent className="p-0">
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('ddns.providerName')}</TableHead>
                      <TableHead>{t('ddns.providerType')}</TableHead>
                      <TableHead>{t('ddns.createdTime')}</TableHead>
                      <TableHead className="text-right">{t('ddns.actions')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {loading ? (
                      <TableRow><TableCell colSpan={4} className="py-8 text-center text-muted-foreground">{t('common.loading')}</TableCell></TableRow>
                    ) : providers.length === 0 ? (
                      <TableRow><TableCell colSpan={4} className="py-8 text-center text-muted-foreground">{t('common.noData')}</TableCell></TableRow>
                    ) : providers.map(provider => (
                      <TableRow key={provider.id}>
                        <TableCell className="font-medium">{provider.name}</TableCell>
                        <TableCell><Badge variant="outline" className="rounded-md">{provider.type}</Badge></TableCell>
                        <TableCell className="text-sm">{formatTime(provider.createdTime)}</TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button variant="outline" size="icon" onClick={() => handleEditProvider(provider)}>
                              <Edit2 className="h-4 w-4" />
                            </Button>
                            <Button variant="destructive" size="icon" onClick={() => handleDeleteProvider(provider)}>
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <Dialog open={providerDialogOpen} onOpenChange={setProviderDialogOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{editingProvider ? t('ddns.editProvider') : t('ddns.createProvider')}</DialogTitle>
          </DialogHeader>

          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t('ddns.providerName')}</Label>
              <Input value={providerForm.name} onChange={e => setProviderForm(prev => ({ ...prev, name: e.target.value }))} />
            </div>
            <div className="space-y-2">
              <Label>{t('ddns.providerType')}</Label>
              <Select
                value={providerForm.type}
                onValueChange={value => setProviderForm(prev => ({ ...prev, type: value }))}
                disabled={!!editingProvider}
              >
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {providerTypes.map(type => (
                    <SelectItem key={type} value={type}>{type}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {providerForm.type === 'cloudflare' && (
              <div className="space-y-2">
                <Label>{t('ddns.apiToken')}</Label>
                <Input
                  type="password"
                  value={providerForm.apiToken}
                  onChange={e => setProviderForm(prev => ({ ...prev, apiToken: e.target.value }))}
                  placeholder={editingProvider ? t('ddns.credentialKeepBlank') : ''}
                />
              </div>
            )}

            {providerForm.type === 'webhook' && (
              <>
                <div className="space-y-2">
                  <Label>{t('ddns.webhookUrl')}</Label>
                  <Input
                    value={providerForm.url}
                    onChange={e => setProviderForm(prev => ({ ...prev, url: e.target.value }))}
                    placeholder={editingProvider ? t('ddns.credentialKeepBlank') : 'https://example.com/ddns?ip={ip}'}
                  />
                </div>
                <div className="space-y-2">
                  <Label>{t('ddns.webhookMethod')}</Label>
                  <Select value={providerForm.method} onValueChange={value => setProviderForm(prev => ({ ...prev, method: value }))}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="GET">GET</SelectItem>
                      <SelectItem value="POST">POST</SelectItem>
                      <SelectItem value="PUT">PUT</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>{t('ddns.webhookBody')}</Label>
                  <Input
                    value={providerForm.body}
                    onChange={e => setProviderForm(prev => ({ ...prev, body: e.target.value }))}
                    placeholder='{"ip":"{ip}"}'
                  />
                </div>
                <p className="text-xs text-muted-foreground">{t('ddns.webhookPlaceholderHint')}</p>
              </>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setProviderDialogOpen(false)}>{t('common.cancel')}</Button>
            <Button onClick={handleSubmitProvider}>{editingProvider ? t('common.update') : t('common.create')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={domainDialogOpen} onOpenChange={setDomainDialogOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{editingDomain ? t('ddns.editDomain') : t('ddns.createDomain')}</DialogTitle>
          </DialogHeader>

          <div className="space-y-4">
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label>{t('ddns.provider')}</Label>
                <Select value={domainForm.providerId} onValueChange={value => setDomainForm(prev => ({ ...prev, providerId: value }))}>
                  <SelectTrigger><SelectValue placeholder={t('ddns.selectProvider')} /></SelectTrigger>
                  <SelectContent>
                    {providers.map(p => (
                      <SelectItem key={p.id} value={String(p.id)}>{p.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{t('ddns.group')}</Label>
                <Select
                  value={domainForm.groupId}
                  onValueChange={value => setDomainForm(prev => ({ ...prev, groupId: value }))}
                  disabled={!!editingDomain}
                >
                  <SelectTrigger><SelectValue placeholder={t('ddns.selectGroup')} /></SelectTrigger>
                  <SelectContent>
                    {groups.map(g => (
                      <SelectItem key={g.id} value={String(g.id)}>{g.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label>{t('ddns.rootDomain')}</Label>
                <Input value={domainForm.domain} onChange={e => setDomainForm(prev => ({ ...prev, domain: e.target.value }))} placeholder="example.com" />
              </div>
              <div className="space-y-2">
                <Label>{t('ddns.recordName')}</Label>
                <Input value={domainForm.recordName} onChange={e => setDomainForm(prev => ({ ...prev, recordName: e.target.value }))} placeholder="@ / vpn" />
              </div>
            </div>

            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label>{t('ddns.recordType')}</Label>
                <Select value={domainForm.recordType} onValueChange={value => setDomainForm(prev => ({ ...prev, recordType: value }))}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="A">A (IPv4)</SelectItem>
                    <SelectItem value="AAAA">AAAA (IPv6)</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="flex items-center gap-3 pt-6">
                <Switch checked={domainForm.autoResolve} onCheckedChange={checked => setDomainForm(prev => ({ ...prev, autoResolve: checked }))} />
                <Label>{t('ddns.autoResolve')}</Label>
              </div>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setDomainDialogOpen(false)}>{t('common.cancel')}</Button>
            <Button onClick={handleSubmitDomain}>{editingDomain ? t('common.update') : t('common.create')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
