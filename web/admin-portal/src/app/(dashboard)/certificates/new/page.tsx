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
import { issueCertificate } from "@/lib/api/certificates";

export default function IssueCertificatePage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState({
    commonName: "",
    altNames: "",
    ipSANs: "",
    ttl: "8760h",
  });

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError(null);
    try {
      const cert = await issueCertificate({
        commonName: form.commonName,
        altNames: form.altNames
          ? form.altNames.split(",").map((s) => s.trim())
          : undefined,
        ipSANs: form.ipSANs
          ? form.ipSANs.split(",").map((s) => s.trim())
          : undefined,
        ttl: form.ttl,
      });

      // Download the certificate immediately after issuance
      const blob = new Blob([cert.certificatePEM], {
        type: "application/x-pem-file",
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${form.commonName}.pem`;
      a.click();
      URL.revokeObjectURL(url);

      router.push("/certificates");
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Failed to issue certificate"
      );
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="max-w-2xl space-y-6">
      <h1 className="text-3xl font-bold">Issue Certificate</h1>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      <form onSubmit={handleSubmit}>
        <Card>
          <CardHeader>
            <CardTitle>Certificate Details</CardTitle>
            <CardDescription>
              Issue a new client certificate via Vault PKI
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <label className="text-sm font-medium" htmlFor="cn">
                Common Name (CN) *
              </label>
              <Input
                id="cn"
                value={form.commonName}
                onChange={(e) =>
                  setForm((f) => ({ ...f, commonName: e.target.value }))
                }
                placeholder="e.g., smo-client.netweave.local"
                required
              />
            </div>
            <div>
              <label className="text-sm font-medium" htmlFor="altNames">
                Alternative DNS Names
              </label>
              <Input
                id="altNames"
                value={form.altNames}
                onChange={(e) =>
                  setForm((f) => ({ ...f, altNames: e.target.value }))
                }
                placeholder="Comma-separated: dns1.example.com, dns2.example.com"
              />
            </div>
            <div>
              <label className="text-sm font-medium" htmlFor="ipSANs">
                IP SANs
              </label>
              <Input
                id="ipSANs"
                value={form.ipSANs}
                onChange={(e) =>
                  setForm((f) => ({ ...f, ipSANs: e.target.value }))
                }
                placeholder="Comma-separated: 10.0.0.1, 192.168.1.1"
              />
            </div>
            <div>
              <label className="text-sm font-medium" htmlFor="ttl">
                TTL (Time to Live)
              </label>
              <select
                id="ttl"
                value={form.ttl}
                onChange={(e) =>
                  setForm((f) => ({ ...f, ttl: e.target.value }))
                }
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="720h">30 days</option>
                <option value="2160h">90 days</option>
                <option value="4380h">6 months</option>
                <option value="8760h">1 year (default)</option>
              </select>
            </div>
          </CardContent>
        </Card>

        <div className="flex gap-4 mt-6">
          <Button type="submit" disabled={loading}>
            {loading ? "Issuing..." : "Issue Certificate"}
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
