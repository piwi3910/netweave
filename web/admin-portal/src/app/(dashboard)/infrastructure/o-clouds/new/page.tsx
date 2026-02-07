"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft, Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { DynamicForm } from "@/components/backends/dynamic-form";
import { CardSkeleton } from "@/components/shared/loading-skeleton";
import { listBackendTypes, createBackend } from "@/lib/api/backends";
import { getDisplayError } from "@/lib/utils/sanitize-error";
import { backendSchema } from "@/lib/validation/schemas";
import type { AdapterTypeSchema } from "@/types/backends";

export default function NewOCloudPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [types, setTypes] = useState<AdapterTypeSchema[]>([]);
  const [selectedType, setSelectedType] = useState<AdapterTypeSchema | null>(
    null
  );
  const [form, setForm] = useState({
    name: "",
    description: "",
  });
  const [configValues, setConfigValues] = useState<Record<string, string>>({});
  const [credentialValues, setCredentialValues] = useState<
    Record<string, string>
  >({});
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    loadTypes();
  }, []);

  async function loadTypes() {
    try {
      setLoading(true);
      const data = await listBackendTypes("ims");
      setTypes(data);
    } catch (err) {
      setError(getDisplayError(err));
    } finally {
      setLoading(false);
    }
  }

  function handleTypeChange(typeValue: string) {
    const type = types.find((t) => t.type === typeValue);
    setSelectedType(type || null);
    setConfigValues({});
    setCredentialValues({});
    setFieldErrors({});
  }

  function validateDynamicFields(): boolean {
    const errors: Record<string, string> = {};

    if (!selectedType) {
      return false;
    }

    for (const field of selectedType.configSchema) {
      const value = configValues[field.name] || "";
      if (field.required && !value) {
        errors[field.name] = `${field.label} is required`;
      } else if (field.regex && value) {
        const regex = new RegExp(field.regex);
        if (!regex.test(value)) {
          errors[field.name] = `Invalid format for ${field.label}`;
        }
      }
    }

    for (const field of selectedType.credentialSchema) {
      const value = credentialValues[field.name] || "";
      if (field.required && !value) {
        errors[field.name] = `${field.label} is required`;
      } else if (field.regex && value) {
        const regex = new RegExp(field.regex);
        if (!regex.test(value)) {
          errors[field.name] = `Invalid format for ${field.label}`;
        }
      }
    }

    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError(null);

    const result = backendSchema.safeParse({
      name: form.name,
      description: form.description,
      category: "ims",
      adapterType: selectedType?.type,
    });

    if (!result.success) {
      const firstError = result.error.errors[0];
      setError(firstError.message);
      setSaving(false);
      return;
    }

    if (!validateDynamicFields()) {
      setError("Please fix the field errors");
      setSaving(false);
      return;
    }

    try {
      const backend = await createBackend({
        name: form.name,
        description: form.description,
        category: "ims",
        adapterType: selectedType!.type,
        config: configValues,
        credentials: credentialValues,
      });
      router.push(`/infrastructure/o-clouds/${backend.id}`);
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

  return (
    <div className="max-w-2xl space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => router.back()}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <h1 className="text-3xl font-bold">Add O-Cloud</h1>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-6">
        <Card>
          <CardHeader>
            <CardTitle>Basic Information</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <label className="text-sm font-medium" htmlFor="name">
                Name <span className="text-destructive">*</span>
              </label>
              <Input
                id="name"
                value={form.name}
                onChange={(e) =>
                  setForm((f) => ({ ...f, name: e.target.value }))
                }
                required
                maxLength={128}
              />
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
                maxLength={512}
              />
            </div>
            <div>
              <label className="text-sm font-medium" htmlFor="type">
                Adapter Type <span className="text-destructive">*</span>
              </label>
              <select
                id="type"
                value={selectedType?.type || ""}
                onChange={(e) => handleTypeChange(e.target.value)}
                required
                className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
              >
                <option value="">Select adapter type...</option>
                {types.map((type) => (
                  <option key={type.type} value={type.type}>
                    {type.displayName}
                  </option>
                ))}
              </select>
              {selectedType && (
                <p className="text-xs text-muted-foreground mt-1">
                  {selectedType.description}
                </p>
              )}
            </div>
          </CardContent>
        </Card>

        {selectedType && selectedType.configSchema.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle>Configuration</CardTitle>
              <CardDescription>
                Configure connection parameters for this backend
              </CardDescription>
            </CardHeader>
            <CardContent>
              <DynamicForm
                fields={selectedType.configSchema}
                values={configValues}
                onChange={(name, value) =>
                  setConfigValues((v) => ({ ...v, [name]: value }))
                }
                errors={fieldErrors}
              />
            </CardContent>
          </Card>
        )}

        {selectedType && selectedType.credentialSchema.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle>Credentials</CardTitle>
              <CardDescription>
                Provide authentication credentials for this backend
              </CardDescription>
            </CardHeader>
            <CardContent>
              <DynamicForm
                fields={selectedType.credentialSchema}
                values={credentialValues}
                onChange={(name, value) =>
                  setCredentialValues((v) => ({ ...v, [name]: value }))
                }
                errors={fieldErrors}
              />
            </CardContent>
          </Card>
        )}

        <div className="flex gap-4">
          <Button type="submit" disabled={saving || !selectedType}>
            <Save className="mr-2 h-4 w-4" />
            {saving ? "Creating..." : "Create O-Cloud"}
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
