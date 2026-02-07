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
import { BackendStatusBadge } from "@/components/backends/backend-status-badge";
import { TestConnectionButton } from "@/components/backends/test-connection-button";
import {
  getBackend,
  updateBackend,
  listDMSLinks,
  createDMSLink,
  deleteDMSLink,
  listBackends,
} from "@/lib/api/backends";
import { getDisplayError } from "@/lib/utils/sanitize-error";
import { formatDate } from "@/lib/utils";
import type { BackendResponse } from "@/types/backends";

export default function OCloudDetailPage() {
  const params = useParams();
  const router = useRouter();
  const backendId = params.id as string;
  const [backend, setBackend] = useState<BackendResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState({
    name: "",
    description: "",
  });
  const [dmsLinks, setDmsLinks] = useState<BackendResponse[]>([]);
  const [allDms, setAllDms] = useState<BackendResponse[]>([]);
  const [showAddDms, setShowAddDms] = useState(false);
  const [selectedDms, setSelectedDms] = useState("");

  async function load() {
    try {
      const [backendData, dmsData, allDmsData] = await Promise.all([
        getBackend(backendId),
        listDMSLinks(backendId),
        listBackends("dms"),
      ]);
      setBackend(backendData);
      setForm({
        name: backendData.name,
        description: backendData.description,
      });
      setDmsLinks(dmsData);
      setAllDms(allDmsData);
    } catch (err) {
      setError(getDisplayError(err));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, [backendId]);

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      const updated = await updateBackend(backendId, form);
      setBackend(updated);
    } catch (err) {
      setError(getDisplayError(err));
    } finally {
      setSaving(false);
    }
  }

  async function handleAddDms() {
    if (!selectedDms) return;
    try {
      await createDMSLink(backendId, selectedDms);
      const dmsData = await listDMSLinks(backendId);
      setDmsLinks(dmsData);
      setSelectedDms("");
      setShowAddDms(false);
    } catch (err) {
      setError(getDisplayError(err));
    }
  }

  async function handleRemoveDms(dmsId: string, dmsName: string) {
    if (!confirm(`Remove DMS link to "${dmsName}"?`)) {
      return;
    }
    try {
      await deleteDMSLink(backendId, dmsId);
      setDmsLinks((prev) => prev.filter((d) => d.id !== dmsId));
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

  if (!backend) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
        <p className="text-sm text-destructive">
          {error || "O-Cloud not found"}
        </p>
      </div>
    );
  }

  const availableDms = allDms.filter(
    (d) => !dmsLinks.some((link) => link.id === d.id)
  );

  return (
    <div className="max-w-2xl space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => router.back()}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-3xl font-bold">{backend.name}</h1>
          <p className="text-sm text-muted-foreground">{backend.id}</p>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <Badge variant="outline">{backend.adapterType}</Badge>
          <BackendStatusBadge status={backend.status} />
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      <form onSubmit={handleSave} className="space-y-6">
        <Card>
          <CardHeader>
            <CardTitle>O-Cloud Details</CardTitle>
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
              <p className="text-sm font-medium mb-1">Status</p>
              <p className="text-sm text-muted-foreground">
                {backend.statusMessage || "No status message"}
              </p>
              <p className="text-xs text-muted-foreground mt-1">
                Last checked: {formatDate(backend.lastHealthCheck)}
              </p>
            </div>
            <div>
              <TestConnectionButton
                backendId={backendId}
                onTestComplete={load}
              />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Linked DMS</CardTitle>
            <CardDescription>
              Deployment management services linked to this O-Cloud
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {dmsLinks.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No DMS linked to this O-Cloud
              </p>
            ) : (
              <div className="space-y-2">
                {dmsLinks.map((dms) => (
                  <div
                    key={dms.id}
                    className="flex items-center justify-between rounded-lg border p-3"
                  >
                    <div>
                      <p className="font-medium">{dms.name}</p>
                      <p className="text-xs text-muted-foreground">
                        <Badge variant="outline" className="text-xs">
                          {dms.adapterType}
                        </Badge>
                      </p>
                    </div>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleRemoveDms(dms.id, dms.name)}
                    >
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  </div>
                ))}
              </div>
            )}
            {!showAddDms && availableDms.length > 0 && (
              <Button
                type="button"
                variant="outline"
                onClick={() => setShowAddDms(true)}
              >
                <Plus className="mr-2 h-4 w-4" />
                Link DMS
              </Button>
            )}
            {showAddDms && (
              <div className="flex gap-2">
                <select
                  value={selectedDms}
                  onChange={(e) => setSelectedDms(e.target.value)}
                  className="flex-1 rounded-md border border-input bg-background px-3 py-2 text-sm"
                >
                  <option value="">Select DMS...</option>
                  {availableDms.map((dms) => (
                    <option key={dms.id} value={dms.id}>
                      {dms.name} ({dms.adapterType})
                    </option>
                  ))}
                </select>
                <Button type="button" onClick={handleAddDms}>
                  Add
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => {
                    setShowAddDms(false);
                    setSelectedDms("");
                  }}
                >
                  Cancel
                </Button>
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
