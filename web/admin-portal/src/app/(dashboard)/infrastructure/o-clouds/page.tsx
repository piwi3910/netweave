"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Cloud, Plus, Trash2, Pencil } from "lucide-react";
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
import { BackendStatusBadge } from "@/components/backends/backend-status-badge";
import { listBackends, deleteBackend } from "@/lib/api/backends";
import { getDisplayError } from "@/lib/utils/sanitize-error";
import type { BackendResponse } from "@/types/backends";
import { formatDate } from "@/lib/utils";

export default function OCloudsPage() {
  const router = useRouter();
  const [backends, setBackends] = useState<BackendResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadBackends();
  }, []);

  async function loadBackends() {
    try {
      setLoading(true);
      const data = await listBackends("ims");
      setBackends(data);
    } catch (err) {
      setError(getDisplayError(err));
    } finally {
      setLoading(false);
    }
  }

  async function handleDelete(backendId: string, name: string) {
    if (!confirm(`Delete O-Cloud "${name}"? This action cannot be undone.`)) {
      return;
    }
    try {
      await deleteBackend(backendId);
      setBackends((prev) => prev.filter((b) => b.id !== backendId));
    } catch (err) {
      setError(getDisplayError(err));
    }
  }

  const filtered = backends.filter(
    (b) =>
      b.name.toLowerCase().includes(search.toLowerCase()) ||
      b.id.toLowerCase().includes(search.toLowerCase()) ||
      b.adapterType.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold">O-Clouds</h1>
        <Button onClick={() => router.push("/infrastructure/o-clouds/new")}>
          <Plus className="mr-2 h-4 w-4" />
          Add O-Cloud
        </Button>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      <Input
        placeholder="Search O-Clouds..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="max-w-sm"
      />

      {loading ? (
        <TableSkeleton />
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={Cloud}
          title="No O-Clouds found"
          description={
            search
              ? "No O-Clouds match your search"
              : "Add your first O-Cloud to get started"
          }
          actionLabel={!search ? "Add O-Cloud" : undefined}
          onAction={
            !search
              ? () => router.push("/infrastructure/o-clouds/new")
              : undefined
          }
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Description</TableHead>
              <TableHead>Created</TableHead>
              <TableHead className="w-24">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.map((backend) => (
              <TableRow key={backend.id}>
                <TableCell>
                  <Link
                    href={`/infrastructure/o-clouds/${backend.id}`}
                    className="font-medium hover:underline"
                  >
                    {backend.name}
                  </Link>
                  <div className="text-xs text-muted-foreground">
                    {backend.id}
                  </div>
                </TableCell>
                <TableCell>
                  <Badge variant="outline">{backend.adapterType}</Badge>
                </TableCell>
                <TableCell>
                  <BackendStatusBadge status={backend.status} />
                </TableCell>
                <TableCell className="text-sm max-w-xs truncate">
                  {backend.description || "-"}
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {formatDate(backend.createdAt)}
                </TableCell>
                <TableCell>
                  <div className="flex gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() =>
                        router.push(`/infrastructure/o-clouds/${backend.id}`)
                      }
                      aria-label="Edit O-Cloud"
                    >
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleDelete(backend.id, backend.name)}
                      aria-label="Delete O-Cloud"
                    >
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
