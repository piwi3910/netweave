"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft, Save, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { CardSkeleton } from "@/components/shared/loading-skeleton";
import { getTenant, updateTenant } from "@/lib/api/tenants";
import {
  listBackendAccess,
  createBackendAccess,
  deleteBackendAccess,
  listBackends,
} from "@/lib/api/backends";
import { getDisplayError } from "@/lib/utils/sanitize-error";
import { formatDate } from "@/lib/utils";
import type { Tenant } from "@/types/api";
import type { BackendAccess, BackendResponse } from "@/types/backends";

export default function TenantDetailPage() {
  const params = useParams();
  const router = useRouter();
  const tenantId = params.id as string;
  const [tenant, setTenant] = useState<Tenant | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState({
    name: "",
    description: "",
    contactEmail: "",
  });
  const [backendAccess, setBackendAccess] = useState<BackendAccess[]>([]);
  const [allBackends, setAllBackends] = useState<BackendResponse[]>([]);
  const [showGrantAccess, setShowGrantAccess] = useState(false);
  const [grantForm, setGrantForm] = useState({
    backendId: "",
    permissions: "read",
  });

  useEffect(() => {
    async function load() {
      try {
        const [tenantData, accessData, backendsData] = await Promise.all([
          getTenant(tenantId),
          listBackendAccess(tenantId),
          Promise.all([listBackends("ims"), listBackends("dms")]).then(
            ([ims, dms]) => [...ims, ...dms]
          ),
        ]);
        setTenant(tenantData);
        setForm({
          name: tenantData.name,
          description: tenantData.description,
          contactEmail: tenantData.contactEmail,
        });
        setBackendAccess(accessData);
        setAllBackends(backendsData);
      } catch (err) {
        setError(getDisplayError(err));
      } finally {
        setLoading(false);
      }
    }
    load();
  }, [tenantId]);

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      const updated = await updateTenant(tenantId, form);
      setTenant(updated);
    } catch (err) {
      setError(getDisplayError(err));
    } finally {
      setSaving(false);
    }
  }

  async function handleGrantAccess() {
    if (!grantForm.backendId) return;
    try {
      const permissions = grantForm.permissions.split(",").map((p) => p.trim());
      const access = await createBackendAccess(tenantId, {
        backendId: grantForm.backendId,
        permissions,
      });
      setBackendAccess((prev) => [...prev, access]);
      setGrantForm({ backendId: "", permissions: "read" });
      setShowGrantAccess(false);
    } catch (err) {
      setError(getDisplayError(err));
    }
  }

  async function handleRevokeAccess(accessId: string, backendName: string) {
    if (!confirm(`Revoke access to "${backendName}"?`)) {
      return;
    }
    try {
      await deleteBackendAccess(tenantId, accessId);
      setBackendAccess((prev) => prev.filter((a) => a.id !== accessId));
    } catch (err) {
      setError(getDisplayError(err));
    }
  }

  if (loading) {
    return (
      <div className="max-w-2xl space-y-6">
        <CardSkeleton />
        <CardSkeleton />
      </div>
    );
  }

  if (!tenant) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
        <p className="text-sm text-destructive">
          {error || "Tenant not found"}
        </p>
      </div>
    );
  }

  return (
    <div className="max-w-2xl space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => router.back()}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-3xl font-bold">{tenant.name}</h1>
          <p className="text-sm text-muted-foreground">{tenant.id}</p>
        </div>
        <Badge
          variant={tenant.status === "active" ? "success" : "warning"}
          className="ml-auto"
        >
          {tenant.status}
        </Badge>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      <form onSubmit={handleSave} className="space-y-6">
        <Card>
          <CardHeader>
            <CardTitle>Tenant Details</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <label className="text-sm font-medium" htmlFor="name">
                Name
              </label>
              <Input
                id="name"
                value={form.name}
                onChange={(e) =>
                  setForm((f) => ({ ...f, name: e.target.value }))
                }
                required
              />
            </div>
            <div>
              <label className="text-sm font-medium" htmlFor="desc">
                Description
              </label>
              <Input
                id="desc"
                value={form.description}
                onChange={(e) =>
                  setForm((f) => ({ ...f, description: e.target.value }))
                }
              />
            </div>
            <div>
              <label className="text-sm font-medium" htmlFor="email">
                Contact Email
              </label>
              <Input
                id="email"
                type="email"
                value={form.contactEmail}
                onChange={(e) =>
                  setForm((f) => ({ ...f, contactEmail: e.target.value }))
                }
              />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Resource Usage</CardTitle>
            <CardDescription>Current usage vs quota limits</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 gap-4">
              <UsageBar
                label="Subscriptions"
                used={tenant.usage.subscriptions}
                max={tenant.quota.maxSubscriptions}
              />
              <UsageBar
                label="Resource Pools"
                used={tenant.usage.resourcePools}
                max={tenant.quota.maxResourcePools}
              />
              <UsageBar
                label="Resources"
                used={tenant.usage.resources}
                max={tenant.quota.maxResources}
              />
              <UsageBar
                label="Users"
                used={tenant.usage.users}
                max={tenant.quota.maxUsers}
              />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Backend Access</CardTitle>
            <CardDescription>
              Infrastructure backends this tenant can access
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {backendAccess.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No backend access granted
              </p>
            ) : (
              <div className="space-y-2">
                {backendAccess.map((access) => {
                  const backend = allBackends.find(
                    (b) => b.id === access.backendId
                  );
                  return (
                    <div
                      key={access.id}
                      className="flex items-center justify-between rounded-lg border p-3"
                    >
                      <div>
                        <p className="font-medium">
                          {backend?.name || access.backendId}
                        </p>
                        <div className="flex gap-2 mt-1">
                          {access.permissions.map((perm) => (
                            <Badge key={perm} variant="secondary" className="text-xs">
                              {perm}
                            </Badge>
                          ))}
                        </div>
                        <p className="text-xs text-muted-foreground mt-1">
                          Granted {formatDate(access.grantedAt)}
                        </p>
                      </div>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() =>
                          handleRevokeAccess(
                            access.id,
                            backend?.name || access.backendId
                          )
                        }
                      >
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    </div>
                  );
                })}
              </div>
            )}
            {!showGrantAccess && (
              <Button
                type="button"
                variant="outline"
                onClick={() => setShowGrantAccess(true)}
              >
                <Plus className="mr-2 h-4 w-4" />
                Grant Access
              </Button>
            )}
            {showGrantAccess && (
              <div className="space-y-3 p-4 border rounded-lg">
                <div>
                  <label className="text-sm font-medium">Backend</label>
                  <select
                    value={grantForm.backendId}
                    onChange={(e) =>
                      setGrantForm((f) => ({ ...f, backendId: e.target.value }))
                    }
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm mt-1"
                  >
                    <option value="">Select backend...</option>
                    {allBackends.map((backend) => (
                      <option key={backend.id} value={backend.id}>
                        {backend.name} ({backend.category})
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="text-sm font-medium">Permissions</label>
                  <Input
                    value={grantForm.permissions}
                    onChange={(e) =>
                      setGrantForm((f) => ({
                        ...f,
                        permissions: e.target.value,
                      }))
                    }
                    placeholder="read, write, delete"
                    className="mt-1"
                  />
                  <p className="text-xs text-muted-foreground mt-1">
                    Comma-separated list of permissions
                  </p>
                </div>
                <div className="flex gap-2">
                  <Button type="button" onClick={handleGrantAccess}>
                    Grant
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => {
                      setShowGrantAccess(false);
                      setGrantForm({ backendId: "", permissions: "read" });
                    }}
                  >
                    Cancel
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        <div className="flex gap-4">
          <Button type="submit" disabled={saving}>
            <Save className="mr-2 h-4 w-4" />
            {saving ? "Saving..." : "Save Changes"}
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => router.back()}
          >
            Cancel
          </Button>
        </div>
      </form>
    </div>
  );
}

function UsageBar({
  label,
  used,
  max,
}: {
  label: string;
  used: number;
  max: number;
}) {
  const pct = max > 0 ? Math.min((used / max) * 100, 100) : 0;
  const color =
    pct >= 90 ? "bg-destructive" : pct >= 70 ? "bg-yellow-500" : "bg-primary";

  return (
    <div>
      <div className="flex justify-between text-sm mb-1">
        <span>{label}</span>
        <span className="text-muted-foreground">
          {used} / {max}
        </span>
      </div>
      <div className="h-2 rounded-full bg-muted">
        <div
          className={`h-full rounded-full ${color}`}
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  );
}
