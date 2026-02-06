"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { createTenant } from "@/lib/api/tenants";
import { tenantSchema } from "@/lib/validation/schemas";
import { getDisplayError } from "@/lib/utils/sanitize-error";

export default function CreateTenantPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [form, setForm] = useState({
    name: "",
    description: "",
    contactEmail: "",
    maxSubscriptions: "100",
    maxResourcePools: "50",
    maxResources: "1000",
    maxUsers: "50",
  });

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError(null);
    setFieldErrors({});

    const result = tenantSchema.safeParse({
      name: form.name,
      description: form.description,
      contactEmail: form.contactEmail,
    });
    if (!result.success) {
      const errors: Record<string, string> = {};
      for (const issue of result.error.issues) {
        const key = issue.path[0]?.toString() ?? "form";
        errors[key] = issue.message;
      }
      setFieldErrors(errors);
      setLoading(false);
      return;
    }

    try {
      await createTenant({
        name: result.data.name,
        description: result.data.description,
        contactEmail: result.data.contactEmail,
        quota: {
          maxSubscriptions: parseInt(form.maxSubscriptions, 10),
          maxResourcePools: parseInt(form.maxResourcePools, 10),
          maxResources: parseInt(form.maxResources, 10),
          maxUsers: parseInt(form.maxUsers, 10),
        },
      });
      router.push("/tenants");
    } catch (err) {
      setError(getDisplayError(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="max-w-2xl space-y-6">
      <h1 className="text-3xl font-bold">Create Tenant</h1>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-6">
        <Card>
          <CardHeader>
            <CardTitle>Tenant Details</CardTitle>
            <CardDescription>Basic tenant information</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <label className="text-sm font-medium" htmlFor="name">
                Name *
              </label>
              <Input
                id="name"
                value={form.name}
                onChange={(e) =>
                  setForm((f) => ({ ...f, name: e.target.value }))
                }
                placeholder="Tenant name"
                required
                maxLength={128}
              />
              {fieldErrors.name && (
                <p className="text-xs text-destructive mt-1">
                  {fieldErrors.name}
                </p>
              )}
            </div>
            <div>
              <label className="text-sm font-medium" htmlFor="description">
                Description
              </label>
              <Input
                id="description"
                value={form.description}
                onChange={(e) =>
                  setForm((f) => ({ ...f, description: e.target.value }))
                }
                placeholder="Optional description"
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
                placeholder="admin@example.com"
              />
              {fieldErrors.contactEmail && (
                <p className="text-xs text-destructive mt-1">
                  {fieldErrors.contactEmail}
                </p>
              )}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Quotas</CardTitle>
            <CardDescription>Resource limits for this tenant</CardDescription>
          </CardHeader>
          <CardContent className="grid grid-cols-2 gap-4">
            <div>
              <label className="text-sm font-medium" htmlFor="maxSubs">
                Max Subscriptions
              </label>
              <Input
                id="maxSubs"
                type="number"
                value={form.maxSubscriptions}
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    maxSubscriptions: e.target.value,
                  }))
                }
                min="1"
              />
            </div>
            <div>
              <label className="text-sm font-medium" htmlFor="maxPools">
                Max Resource Pools
              </label>
              <Input
                id="maxPools"
                type="number"
                value={form.maxResourcePools}
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    maxResourcePools: e.target.value,
                  }))
                }
                min="1"
              />
            </div>
            <div>
              <label className="text-sm font-medium" htmlFor="maxRes">
                Max Resources
              </label>
              <Input
                id="maxRes"
                type="number"
                value={form.maxResources}
                onChange={(e) =>
                  setForm((f) => ({ ...f, maxResources: e.target.value }))
                }
                min="1"
              />
            </div>
            <div>
              <label className="text-sm font-medium" htmlFor="maxUsers">
                Max Users
              </label>
              <Input
                id="maxUsers"
                type="number"
                value={form.maxUsers}
                onChange={(e) =>
                  setForm((f) => ({ ...f, maxUsers: e.target.value }))
                }
                min="1"
              />
            </div>
          </CardContent>
        </Card>

        <div className="flex gap-4">
          <Button type="submit" disabled={loading}>
            {loading ? "Creating..." : "Create Tenant"}
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
