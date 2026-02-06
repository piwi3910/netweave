"use client";

import { useEffect, useState, useCallback } from "react";
import { ScrollText, Download, ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { EmptyState } from "@/components/shared/empty-state";
import { listAuditEvents } from "@/lib/api/audit";
import { getDisplayError } from "@/lib/utils/sanitize-error";
import type { AuditEvent } from "@/types/api";
import { formatDateTime } from "@/lib/utils";

const PAGE_SIZE = 25;

export default function AuditPage() {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [offset, setOffset] = useState(0);

  const loadEvents = useCallback(async () => {
    try {
      setLoading(true);
      const data = await listAuditEvents({
        limit: PAGE_SIZE,
        offset,
      });
      setEvents(data);
    } catch (err) {
      setError(getDisplayError(err));
    } finally {
      setLoading(false);
    }
  }, [offset]);

  useEffect(() => {
    loadEvents();
  }, [loadEvents]);

  function handleExport() {
    const csv = [
      "Timestamp,Action,Resource Type,Resource ID,User,IP Address",
      ...events.map(
        (e) =>
          `${e.timestamp},${e.action},${e.resourceType},${e.resourceId},${e.subject},${e.clientIP}`
      ),
    ].join("\n");
    const blob = new Blob([csv], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `audit-log-${new Date().toISOString().split("T")[0]}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  const filtered = events.filter(
    (e) =>
      !search ||
      e.action.toLowerCase().includes(search.toLowerCase()) ||
      e.resourceType.toLowerCase().includes(search.toLowerCase()) ||
      e.subject.toLowerCase().includes(search.toLowerCase())
  );

  const actionColor = (action: string) => {
    switch (action) {
      case "create":
        return "success" as const;
      case "delete":
        return "destructive" as const;
      case "update":
        return "warning" as const;
      default:
        return "secondary" as const;
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold">Audit Log</h1>
        <Button variant="outline" onClick={handleExport}>
          <Download className="mr-2 h-4 w-4" />
          Export CSV
        </Button>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      <Input
        placeholder="Search events..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="max-w-sm"
      />

      {loading ? (
        <TableSkeleton rows={10} />
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={ScrollText}
          title="No audit events found"
          description={
            search
              ? "No events match your search"
              : "No audit events recorded yet"
          }
        />
      ) : (
        <>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Timestamp</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Resource</TableHead>
                <TableHead>User</TableHead>
                <TableHead>IP Address</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((event) => (
                <TableRow key={event.id}>
                  <TableCell className="text-sm text-muted-foreground whitespace-nowrap">
                    {formatDateTime(event.timestamp)}
                  </TableCell>
                  <TableCell>
                    <Badge variant={actionColor(event.action)}>
                      {event.action}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="text-sm">
                      <span className="font-medium">
                        {event.resourceType}
                      </span>
                      <div className="text-xs text-muted-foreground font-mono truncate max-w-[200px]">
                        {event.resourceId}
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="text-sm">{event.subject}</TableCell>
                  <TableCell className="text-sm text-muted-foreground font-mono">
                    {event.clientIP}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>

          <div className="flex items-center justify-between">
            <p className="text-sm text-muted-foreground">
              Showing {offset + 1} - {offset + filtered.length}
            </p>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={offset === 0}
                onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
              >
                <ChevronLeft className="h-4 w-4 mr-1" />
                Previous
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={events.length < PAGE_SIZE}
                onClick={() => setOffset((o) => o + PAGE_SIZE)}
              >
                Next
                <ChevronRight className="h-4 w-4 ml-1" />
              </Button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
