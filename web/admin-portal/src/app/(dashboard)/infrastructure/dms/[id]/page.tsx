"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft, Save } from "lucide-react";
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
import { getBackend, updateBackend } from "@/lib/api/backends";
import { getDisplayError } from "@/lib/utils/sanitize-error";
import { formatDate } from "@/lib/utils";
import type { BackendResponse } from "@/types/backends";

export default function DMSDetailPage() {
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

  async function load() {
    try {
      const backendData = await getBackend(backendId);
      setBackend(backendData);
      setForm({
        name: backendData.name,
        description: backendData.description,
      });
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

  if (loading) {
    return (
      <div className="max-w-2xl space-y-6">
        <CardSkeleton />
      </div>
    );
  }

  if (!backend) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
        <p className="text-sm text-destructive">{error || "DMS not found"}</p>
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
            <CardTitle>DMS Details</CardTitle>
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
