'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Plus, Edit2, Trash2, X, ChevronUp, ChevronDown } from 'lucide-react';
import { toast } from 'sonner';
import {
  createGroup, deleteGroup, getGroupList, updateGroup,
  type NodeGroup,
} from '@/lib/api/group';
import { getNodeList } from '@/lib/api/node';
import { useAuth } from '@/lib/hooks/use-auth';
import { useTranslation } from '@/lib/i18n';

type NodeItem = {
  id: number;
  name: string;
  serverIp?: string;
};

type MemberForm = {
  nodeId: string;
  memberIp: string;
};

type GroupForm = {
  name: string;
  type: string;
  switchBack: boolean;
  members: MemberForm[];
};

const emptyForm: GroupForm = {
  name: '',
  type: 'entry',
  switchBack: true,
  members: [{ nodeId: '', memberIp: '' }, { nodeId: '', memberIp: '' }],
};

export default function GroupPage() {
  const { isAdmin } = useAuth();
  const { t } = useTranslation();
  const [groups, setGroups] = useState<NodeGroup[]>([]);
  const [nodes, setNodes] = useState<NodeItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<NodeGroup | null>(null);
  const [form, setForm] = useState<GroupForm>(emptyForm);

  const nodeById = useMemo(() => {
    const map = new Map<number, NodeItem>();
    for (const node of nodes) map.set(node.id, node);
    return map;
  }, [nodes]);

  const loadData = useCallback(async () => {
    try {
      const [groupRes, nodeRes] = await Promise.all([getGroupList(), getNodeList()]);
      if (groupRes.code === 0) setGroups(groupRes.data || []);
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
    setEditing(null);
    setForm(emptyForm);
    setDialogOpen(true);
  };

  const handleEdit = (group: NodeGroup) => {
    const members = [...(group.members || [])].sort((a, b) => a.priority - b.priority);
    setEditing(group);
    setForm({
      name: group.name || '',
      type: group.type || 'entry',
      switchBack: group.switchBack === 1,
      members: members.length >= 2
        ? members.map(m => ({ nodeId: String(m.nodeId), memberIp: m.memberIp || '' }))
        : [{ nodeId: '', memberIp: '' }, { nodeId: '', memberIp: '' }],
    });
    setDialogOpen(true);
  };

  const updateMember = (index: number, patch: Partial<MemberForm>) => {
    setForm(prev => ({
      ...prev,
      members: prev.members.map((m, i) => (i === index ? { ...m, ...patch } : m)),
    }));
  };

  const addMember = () => {
    setForm(prev => ({ ...prev, members: [...prev.members, { nodeId: '', memberIp: '' }] }));
  };

  const removeMember = (index: number) => {
    setForm(prev => ({ ...prev, members: prev.members.filter((_, i) => i !== index) }));
  };

  const moveMember = (index: number, delta: number) => {
    setForm(prev => {
      const target = index + delta;
      if (target < 0 || target >= prev.members.length) return prev;
      const members = [...prev.members];
      [members[index], members[target]] = [members[target], members[index]];
      return { ...prev, members };
    });
  };

  const handleSubmit = async () => {
    const nodeIds = form.members.map(m => parseInt(m.nodeId, 10)).filter(Boolean);
    if (!form.name.trim() || nodeIds.length < 2 || nodeIds.length !== form.members.length) {
      toast.error(t('common.fillRequired'));
      return;
    }
    if (new Set(nodeIds).size !== nodeIds.length) {
      toast.error(t('group.uniqueNodes'));
      return;
    }

    const members = form.members.map((m, index) => ({
      nodeId: parseInt(m.nodeId, 10),
      memberIp: m.memberIp.trim(),
      priority: index,
    }));

    const res = editing
      ? await updateGroup({ id: editing.id, name: form.name.trim(), switchBack: form.switchBack ? 1 : 0, members })
      : await createGroup({ name: form.name.trim(), type: form.type, switchBack: form.switchBack ? 1 : 0, members });

    if (res.code === 0) {
      toast.success(editing ? t('common.updateSuccess') : t('common.createSuccess'));
      setDialogOpen(false);
      await loadData();
    } else {
      toast.error(res.msg);
    }
  };

  const handleDelete = async (group: NodeGroup) => {
    if (!confirm(t('group.confirmDelete', { name: group.name }))) return;
    const res = await deleteGroup(group.id);
    if (res.code === 0) {
      toast.success(t('common.deleteSuccess'));
      await loadData();
    } else {
      toast.error(res.msg);
    }
  };

  const typeLabel = (type: string) => {
    if (type === 'entry') return t('group.typeEntry');
    if (type === 'exit') return t('group.typeExit');
    if (type === 'forward') return t('group.typeForward');
    return type;
  };

  const nodeLabel = (nodeId: number, fallback?: string) =>
    nodeById.get(nodeId)?.name || fallback || `#${nodeId}`;

  const renderMembers = (group: NodeGroup) => {
    const members = [...(group.members || [])].sort((a, b) => a.priority - b.priority);
    if (members.length === 0) return <span className="text-muted-foreground">-</span>;
    return (
      <div className="flex flex-wrap items-center gap-1.5">
        {members.map((m, index) => {
          const isActive = group.activeMemberId === m.nodeId;
          return (
            <Badge
              key={`${group.id}-${m.nodeId}`}
              variant={isActive ? 'default' : 'secondary'}
              className="rounded-md gap-1"
            >
              <span className={`inline-block h-1.5 w-1.5 rounded-full ${m.online ? 'bg-green-500' : 'bg-muted-foreground'}`} />
              {index + 1}. {nodeLabel(m.nodeId, m.nodeName)}
              {isActive && <span className="text-[10px] uppercase">{t('group.active')}</span>}
            </Badge>
          );
        })}
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
      <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-2xl font-bold">{t('group.title')}</h2>
          <p className="text-sm text-muted-foreground">{t('group.description')}</p>
        </div>
        <Button onClick={handleCreate}><Plus className="mr-2 h-4 w-4" />{t('group.createGroup')}</Button>
      </div>

      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('group.name')}</TableHead>
                  <TableHead>{t('group.type')}</TableHead>
                  <TableHead>{t('group.members')}</TableHead>
                  <TableHead>{t('group.switchBack')}</TableHead>
                  <TableHead className="text-right">{t('group.actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {loading ? (
                  <TableRow><TableCell colSpan={5} className="py-8 text-center text-muted-foreground">{t('common.loading')}</TableCell></TableRow>
                ) : groups.length === 0 ? (
                  <TableRow><TableCell colSpan={5} className="py-8 text-center text-muted-foreground">{t('common.noData')}</TableCell></TableRow>
                ) : groups.map(group => (
                  <TableRow key={group.id}>
                    <TableCell className="font-medium">{group.name}</TableCell>
                    <TableCell><Badge variant="outline" className="rounded-md">{typeLabel(group.type)}</Badge></TableCell>
                    <TableCell className="min-w-64">{renderMembers(group)}</TableCell>
                    <TableCell>
                      <Badge variant={group.switchBack === 1 ? 'default' : 'secondary'} className="rounded-md">
                        {group.switchBack === 1 ? t('common.enabled') : t('common.disabled')}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        <Button variant="outline" size="icon" onClick={() => handleEdit(group)}>
                          <Edit2 className="h-4 w-4" />
                        </Button>
                        <Button variant="destructive" size="icon" onClick={() => handleDelete(group)}>
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

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{editing ? t('group.editGroup') : t('group.createGroup')}</DialogTitle>
          </DialogHeader>

          <div className="space-y-4">
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label>{t('group.name')}</Label>
                <Input value={form.name} onChange={e => setForm(prev => ({ ...prev, name: e.target.value }))} />
              </div>
              <div className="space-y-2">
                <Label>{t('group.type')}</Label>
                <Select
                  value={form.type}
                  onValueChange={value => setForm(prev => ({ ...prev, type: value }))}
                  disabled={!!editing}
                >
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="entry">{t('group.typeEntry')}</SelectItem>
                    <SelectItem value="exit">{t('group.typeExit')}</SelectItem>
                    <SelectItem value="forward">{t('group.typeForward')}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="flex items-center gap-3">
              <Switch
                checked={form.switchBack}
                onCheckedChange={checked => setForm(prev => ({ ...prev, switchBack: checked }))}
              />
              <div>
                <Label>{t('group.switchBack')}</Label>
                <p className="text-xs text-muted-foreground">{t('group.switchBackDesc')}</p>
              </div>
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <div>
                  <Label>{t('group.members')}</Label>
                  <p className="text-xs text-muted-foreground">{t('group.membersDesc')}</p>
                </div>
                <Button type="button" variant="outline" size="sm" onClick={addMember}>
                  <Plus className="mr-2 h-4 w-4" />{t('group.addMember')}
                </Button>
              </div>
              <div className="space-y-2">
                {form.members.map((member, index) => {
                  const selected = new Set(form.members.filter((_, i) => i !== index).map(m => m.nodeId).filter(Boolean));
                  return (
                    <div key={index} className="grid grid-cols-[auto_1fr_1fr_auto] items-center gap-2">
                      <div className="flex flex-col">
                        <Button type="button" variant="ghost" size="icon" className="h-5 w-6" onClick={() => moveMember(index, -1)} disabled={index === 0}>
                          <ChevronUp className="h-4 w-4" />
                        </Button>
                        <Button type="button" variant="ghost" size="icon" className="h-5 w-6" onClick={() => moveMember(index, 1)} disabled={index === form.members.length - 1}>
                          <ChevronDown className="h-4 w-4" />
                        </Button>
                      </div>
                      <Select value={member.nodeId} onValueChange={value => updateMember(index, { nodeId: value })}>
                        <SelectTrigger><SelectValue placeholder={t('group.selectNode')} /></SelectTrigger>
                        <SelectContent>
                          {nodes.map(node => (
                            <SelectItem key={node.id} value={String(node.id)} disabled={selected.has(String(node.id))}>
                              {node.name}{node.serverIp ? ` (${node.serverIp})` : ''}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <Input
                        value={member.memberIp}
                        onChange={e => updateMember(index, { memberIp: e.target.value })}
                        placeholder={t('group.memberIpPlaceholder')}
                      />
                      <Button type="button" variant="outline" size="icon" onClick={() => removeMember(index)} disabled={form.members.length <= 2}>
                        <X className="h-4 w-4" />
                      </Button>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>{t('common.cancel')}</Button>
            <Button onClick={handleSubmit}>{editing ? t('common.update') : t('common.create')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
