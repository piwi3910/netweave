"use client";

import { useState } from "react";
import { CheckCircle2, XCircle, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { testBackend } from "@/lib/api/backends";
import { getDisplayError } from "@/lib/utils/sanitize-error";

interface TestConnectionButtonProps {
  backendId: string;
  onTestComplete?: () => void;
}

export function TestConnectionButton({
  backendId,
  onTestComplete,
}: TestConnectionButtonProps) {
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState<{
    status: string;
    message: string;
  } | null>(null);

  async function handleTest() {
    setTesting(true);
    setResult(null);
    try {
      const res = await testBackend(backendId);
      setResult({ status: res.status, message: res.message });
      if (onTestComplete) {
        onTestComplete();
      }
    } catch (err) {
      setResult({ status: "error", message: getDisplayError(err) });
    } finally {
      setTesting(false);
    }
  }

  return (
    <div className="space-y-2">
      <Button
        type="button"
        variant="outline"
        onClick={handleTest}
        disabled={testing}
      >
        {testing ? (
          <>
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            Testing...
          </>
        ) : (
          "Test Connection"
        )}
      </Button>

      {result && (
        <div
          className={`flex items-start gap-2 rounded-md border p-3 text-sm ${
            result.status === "healthy"
              ? "border-green-500/50 bg-green-500/10 text-green-700"
              : "border-destructive/50 bg-destructive/10 text-destructive"
          }`}
        >
          {result.status === "healthy" ? (
            <CheckCircle2 className="h-4 w-4 mt-0.5 shrink-0" />
          ) : (
            <XCircle className="h-4 w-4 mt-0.5 shrink-0" />
          )}
          <div>
            <p className="font-medium">
              {result.status === "healthy" ? "Connection successful" : "Connection failed"}
            </p>
            <p className="text-xs opacity-90">{result.message}</p>
          </div>
        </div>
      )}
    </div>
  );
}
