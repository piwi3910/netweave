"use client";

import { useEffect, useState } from "react";
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
import { createUser } from "@/lib/api/users";
import { listRoles } from "@/lib/api/roles";
import { userSchema } from "@/lib/validation/schemas";
import { getDisplayError } from "@/lib/utils/sanitize-error";
import type { Role } from "@/types/api";

export default function CreateUserPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [roles, setRoles] = useState<Role[]>([]);
  const [form, setForm] = useState({
    subject: "",
    commonName: "",
    email: "",
    roleId: "",
  });

  useEffect(() => {
    listRoles()
      .then((r) => setRoles(r.filter((role) => role.type === "tenant")))
      .catch(() => {});
  }, []);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError(null);
    setFieldErrors({});

    const result = userSchema.safeParse(form);
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
      await createUser({
        subject: result.data.subject,
        commonName: result.data.commonName,
        email: result.data.email || undefined,
        roleId: result.data.roleId,
      });
      router.push("/users");
    } catch (err) {
      setError(getDisplayError(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="max-w-2xl space-y-6">
      <h1 className="text-3xl font-bold">Create User</h1>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      <form onSubmit={handleSubmit}>
        <Card>
          <CardHeader>
            <CardTitle>User Details</CardTitle>
            <CardDescription>
              Create a new user in the current tenant
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <label className="text-sm font-medium" htmlFor="subject">
                Keycloak Subject *
              </label>
              <Input
                id="subject"
                value={form.subject}
                onChange={(e) =>
                  setForm((f) => ({ ...f, subject: e.target.value }))
                }
                placeholder="Keycloak user ID"
                required
              />
              {fieldErrors.subject && (
                <p className="text-xs text-destructive mt-1">
                  {fieldErrors.subject}
                </p>
              )}
            </div>
            <div>
              <label className="text-sm font-medium" htmlFor="name">
                Display Name *
              </label>
              <Input
                id="name"
                value={form.commonName}
                onChange={(e) =>
                  setForm((f) => ({ ...f, commonName: e.target.value }))
                }
                placeholder="Full name"
                required
              />
              {fieldErrors.commonName && (
                <p className="text-xs text-destructive mt-1">
                  {fieldErrors.commonName}
                </p>
              )}
            </div>
            <div>
              <label className="text-sm font-medium" htmlFor="email">
                Email
              </label>
              <Input
                id="email"
                type="email"
                value={form.email}
                onChange={(e) =>
                  setForm((f) => ({ ...f, email: e.target.value }))
                }
                placeholder="user@example.com"
              />
              {fieldErrors.email && (
                <p className="text-xs text-destructive mt-1">
                  {fieldErrors.email}
                </p>
              )}
            </div>
            <div>
              <label className="text-sm font-medium" htmlFor="role">
                Role *
              </label>
              <select
                id="role"
                value={form.roleId}
                onChange={(e) =>
                  setForm((f) => ({ ...f, roleId: e.target.value }))
                }
                required
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="">Select a role</option>
                {roles.map((role) => (
                  <option key={role.id} value={role.id}>
                    {role.name}
                  </option>
                ))}
              </select>
              {fieldErrors.roleId && (
                <p className="text-xs text-destructive mt-1">
                  {fieldErrors.roleId}
                </p>
              )}
            </div>
          </CardContent>
        </Card>

        <div className="flex gap-4 mt-6">
          <Button type="submit" disabled={loading}>
            {loading ? "Creating..." : "Create User"}
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
