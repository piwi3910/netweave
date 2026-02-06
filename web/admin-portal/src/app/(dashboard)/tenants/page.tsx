"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Building2, Plus, Trash2, Pencil } from "lucide-react";
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
import { listTenants, deleteTenant } from "@/lib/api/tenants";
import type { Tenant } from "@/types/api";
import { formatDate } from "@/lib/utils";

export default function TenantsPage() {
  const router = useRouter();
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadTenants();
  }, []);

  async function loadTenants() {
    try {
      setLoading(true);
      const data = await listTenants();
      setTenants(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load tenants");
    } finally {
      setLoading(false);
    }
  }

  async function handleDelete(tenantId: string, name: string) {
    if (!confirm(`Delete tenant "${name}"? This action cannot be undone.`)) {
      return;
    }
    try {
      await deleteTenant(tenantId);
      setTenants((prev) => prev.filter((t) => t.id !== tenantId));
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Failed to delete tenant"
      );
    }
  }

  const filtered = tenants.filter(
    (t) =>
      t.name.toLowerCase().includes(search.toLowerCase()) ||
      t.id.toLowerCase().includes(search.toLowerCase())
  );

  const statusVariant = (status: string) => {
    switch (status) {
      case "active":
        return "success" as const;
      case "pending_deletion":
        return "warning" as const;
      case "inactive":
        return "secondary" as const;
      default:
        return "outline" as const;
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold">Tenants</h1>
        <Button onClick={() => router.push("/tenants/new")}>
          <Plus className="mr-2 h-4 w-4" />
          Create Tenant
        </Button>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      <Input
        placeholder="Search tenants..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="max-w-sm"
      />

      {loading ? (
        <TableSkeleton />
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={Building2}
          title="No tenants found"
          description={
            search
              ? "No tenants match your search"
              : "Create your first tenant to get started"
          }
          actionLabel={!search ? "Create Tenant" : undefined}
          onAction={
            !search ? () => router.push("/tenants/new") : undefined
          }
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Contact</TableHead>
              <TableHead>Usage</TableHead>
              <TableHead>Created</TableHead>
              <TableHead className="w-24">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.map((tenant) => (
              <TableRow key={tenant.id}>
                <TableCell>
                  <Link
                    href={`/tenants/${tenant.id}`}
                    className="font-medium hover:underline"
                  >
                    {tenant.name}
                  </Link>
                  <div className="text-xs text-muted-foreground">
                    {tenant.id}
                  </div>
                </TableCell>
                <TableCell>
                  <Badge variant={statusVariant(tenant.status)}>
                    {tenant.status}
                  </Badge>
                </TableCell>
                <TableCell className="text-sm">
                  {tenant.contactEmail || "-"}
                </TableCell>
                <TableCell className="text-sm">
                  {tenant.usage.users}/{tenant.quota.maxUsers} users
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {formatDate(tenant.createdAt)}
                </TableCell>
                <TableCell>
                  <div className="flex gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => router.push(`/tenants/${tenant.id}`)}
                      aria-label="Edit tenant"
                    >
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleDelete(tenant.id, tenant.name)}
                      aria-label="Delete tenant"
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
