"use client";

import { useState } from "react";
import { Eye, EyeOff } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import type { FieldSpec } from "@/types/backends";

interface DynamicFormProps {
  fields: FieldSpec[];
  values: Record<string, string>;
  onChange: (name: string, value: string) => void;
  errors: Record<string, string>;
}

export function DynamicForm({
  fields,
  values,
  onChange,
  errors,
}: DynamicFormProps) {
  const [showSecrets, setShowSecrets] = useState<Record<string, boolean>>({});

  const toggleSecret = (name: string) => {
    setShowSecrets((prev) => ({ ...prev, [name]: !prev[name] }));
  };

  const renderField = (field: FieldSpec) => {
    const value = values[field.name] || field.default || "";
    const error = errors[field.name];
    const isSecret = field.secret;
    const showSecret = showSecrets[field.name];

    switch (field.type) {
      case "boolean":
        return (
          <div key={field.name} className="space-y-2">
            <label className="flex items-center gap-2 text-sm font-medium">
              <input
                type="checkbox"
                checked={value === "true"}
                onChange={(e) =>
                  onChange(field.name, e.target.checked ? "true" : "false")
                }
                className="h-4 w-4 rounded border-gray-300"
              />
              {field.label}
              {field.required && (
                <span className="text-destructive">*</span>
              )}
            </label>
            {field.description && (
              <p className="text-xs text-muted-foreground">
                {field.description}
              </p>
            )}
            {error && (
              <p className="text-xs text-destructive">{error}</p>
            )}
          </div>
        );

      case "number":
        return (
          <div key={field.name} className="space-y-2">
            <label className="text-sm font-medium" htmlFor={field.name}>
              {field.label}
              {field.required && (
                <span className="text-destructive">*</span>
              )}
            </label>
            <Input
              id={field.name}
              type="number"
              value={value}
              onChange={(e) => onChange(field.name, e.target.value)}
              placeholder={field.placeholder}
              required={field.required}
            />
            {field.description && (
              <p className="text-xs text-muted-foreground">
                {field.description}
              </p>
            )}
            {error && (
              <p className="text-xs text-destructive">{error}</p>
            )}
          </div>
        );

      case "text":
        return (
          <div key={field.name} className="space-y-2">
            <label className="text-sm font-medium" htmlFor={field.name}>
              {field.label}
              {field.required && (
                <span className="text-destructive">*</span>
              )}
            </label>
            {isSecret ? (
              <div className="relative">
                <textarea
                  id={field.name}
                  value={value}
                  onChange={(e) => onChange(field.name, e.target.value)}
                  placeholder={field.placeholder}
                  required={field.required}
                  rows={4}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
                  style={
                    {
                      fontFamily: showSecret ? "monospace" : "inherit",
                      WebkitTextSecurity: showSecret ? "none" : "disc",
                    } as React.CSSProperties
                  }
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="absolute right-2 top-2 h-6 w-6"
                  onClick={() => toggleSecret(field.name)}
                >
                  {showSecret ? (
                    <EyeOff className="h-4 w-4" />
                  ) : (
                    <Eye className="h-4 w-4" />
                  )}
                </Button>
              </div>
            ) : (
              <textarea
                id={field.name}
                value={value}
                onChange={(e) => onChange(field.name, e.target.value)}
                placeholder={field.placeholder}
                required={field.required}
                rows={4}
                className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
              />
            )}
            {field.description && (
              <p className="text-xs text-muted-foreground">
                {field.description}
              </p>
            )}
            {error && (
              <p className="text-xs text-destructive">{error}</p>
            )}
          </div>
        );

      default:
        return (
          <div key={field.name} className="space-y-2">
            <label className="text-sm font-medium" htmlFor={field.name}>
              {field.label}
              {field.required && (
                <span className="text-destructive">*</span>
              )}
            </label>
            {isSecret ? (
              <div className="relative">
                <Input
                  id={field.name}
                  type={showSecret ? "text" : "password"}
                  value={value}
                  onChange={(e) => onChange(field.name, e.target.value)}
                  placeholder={field.placeholder}
                  required={field.required}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="absolute right-0 top-0 h-full"
                  onClick={() => toggleSecret(field.name)}
                >
                  {showSecret ? (
                    <EyeOff className="h-4 w-4" />
                  ) : (
                    <Eye className="h-4 w-4" />
                  )}
                </Button>
              </div>
            ) : (
              <Input
                id={field.name}
                type="text"
                value={value}
                onChange={(e) => onChange(field.name, e.target.value)}
                placeholder={field.placeholder}
                required={field.required}
              />
            )}
            {field.description && (
              <p className="text-xs text-muted-foreground">
                {field.description}
              </p>
            )}
            {error && (
              <p className="text-xs text-destructive">{error}</p>
            )}
          </div>
        );
    }
  };

  return <div className="space-y-4">{fields.map(renderField)}</div>;
}
