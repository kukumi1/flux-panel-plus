'use client';

import { useCallback, useEffect, useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { getSystemLogs, type SystemLog } from '@/lib/api/system-log';
import { useAuth } from '@/lib/hooks/use-auth';
import { useTranslation } from '@/lib/i18n';
import { RotateCcw } from 'lucide-react';
import { toast } from 'sonner';

function formatTime(ts: number) {
  if (!ts) return '-';
  return new Date(ts).toLocaleString();
}

export default function LogsPage() {
  const { isAdmin } = useAuth();
  const { t } = useTranslation();
  const [type, setType] = useState('all');
  const [logs, setLogs] = useState<SystemLog[]>([]);
  const [loading, setLoading] = useState(true);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const res = await getSystemLogs({ type: type === 'all' ? '' : type });
      if (res.code === 0) {
        setLogs(res.data || []);
      } else {
        toast.error(res.msg || t('common.loadFailed'));
      }
    } catch {
      toast.error(t('common.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [type, t]);

  useEffect(() => { void loadData(); }, [loadData]);

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
          <h2 className="text-2xl font-bold">{t('logs.title')}</h2>
          <p className="text-sm text-muted-foreground">{t('logs.description')}</p>
        </div>
        <div className="flex items-end gap-2">
          <div className="space-y-2">
            <Label>{t('logs.type')}</Label>
            <Select value={type} onValueChange={setType}>
              <SelectTrigger className="w-40"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('logs.allTypes')}</SelectItem>
                <SelectItem value="ddns">{t('logs.typeDdns')}</SelectItem>
                <SelectItem value="failover">{t('logs.typeFailover')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <Button variant="outline" onClick={() => void loadData()} disabled={loading}>
            <RotateCcw className="mr-2 h-4 w-4" />
            {t('common.refresh')}
          </Button>
        </div>
      </div>

      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-48">{t('logs.time')}</TableHead>
                  <TableHead className="w-28">{t('logs.type')}</TableHead>
                  <TableHead className="w-24">{t('logs.level')}</TableHead>
                  <TableHead>{t('logs.message')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {loading ? (
                  <TableRow><TableCell colSpan={4} className="py-8 text-center text-muted-foreground">{t('common.loading')}</TableCell></TableRow>
                ) : logs.length === 0 ? (
                  <TableRow><TableCell colSpan={4} className="py-8 text-center text-muted-foreground">{t('common.noData')}</TableCell></TableRow>
                ) : logs.map(log => (
                  <TableRow key={log.id}>
                    <TableCell className="whitespace-nowrap text-sm">{formatTime(log.createdTime)}</TableCell>
                    <TableCell><Badge variant="outline" className="rounded-md">{log.type}</Badge></TableCell>
                    <TableCell>
                      <Badge variant={log.level === 'error' ? 'destructive' : 'secondary'} className="rounded-md">
                        {log.level}
                      </Badge>
                    </TableCell>
                    <TableCell className="whitespace-normal">{log.message}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
