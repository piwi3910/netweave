"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft, Download, Ban } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { CardSkeleton } from "@/components/shared/loading-skeleton";
import { getCertificate, revokeCertificate } from "@/lib/api/certificates";
import { getDisplayError } from "@/lib/utils/sanitize-error";
import type { Certificate } from "@/types/api";
import { formatDateTime, daysUntil } from "@/lib/utils";

export default function CertificateDetailPage() {
  const params = useParams();
  const router = useRouter();
  const serial = params.serialNumber as string;
  const [cert, setCert] = useState<Certificate | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getCertificate(serial)
      .then(setCert)
      .catch((err) =>
        setError(getDisplayError(err))
      )
      .finally(() => setLoading(false));
  }, [serial]);

  async function handleRevoke() {
    if (!cert) return;
    if (
      !confirm(
        `Revoke certificate "${cert.commonName}"? This action cannot be undone.`
      )
    ) {
      return;
    }
    try {
      await revokeCertificate(cert.serialNumber);
      setCert((c) => (c ? { ...c, status: "revoked" } : c));
    } catch (err) {
      setError(getDisplayError(err));
    }
  }

  if (loading) {
    return (
      <div className="max-w-2xl">
        <CardSkeleton />
      </div>
    );
  }

  if (!cert) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
        <p className="text-sm text-destructive">
          {error || "Certificate not found"}
        </p>
      </div>
    );
  }

  const days = daysUntil(cert.expiresAt);
  const statusVariant =
    cert.status === "active"
      ? ("success" as const)
      : cert.status === "expiring_soon"
        ? ("warning" as const)
        : ("destructive" as const);

  return (
    <div className="max-w-2xl space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => router.back()}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-3xl font-bold">{cert.commonName}</h1>
          <p className="text-sm text-muted-foreground font-mono">
            {cert.serialNumber}
          </p>
        </div>
        <Badge variant={statusVariant} className="ml-auto">
          {cert.status.replace("_", " ")}
        </Badge>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Certificate Details</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <dt className="font-medium text-muted-foreground">
                Common Name
              </dt>
              <dd>{cert.commonName}</dd>
            </div>
            <div>
              <dt className="font-medium text-muted-foreground">Status</dt>
              <dd>
                <Badge variant={statusVariant}>
                  {cert.status.replace("_", " ")}
                </Badge>
              </dd>
            </div>
            <div>
              <dt className="font-medium text-muted-foreground">Issued At</dt>
              <dd>{formatDateTime(cert.issuedAt)}</dd>
            </div>
            <div>
              <dt className="font-medium text-muted-foreground">Expires At</dt>
              <dd>
                {formatDateTime(cert.expiresAt)}
                {days > 0 && (
                  <span className="text-muted-foreground ml-1">
                    ({days} days remaining)
                  </span>
                )}
              </dd>
            </div>
            <div>
              <dt className="font-medium text-muted-foreground">TTL</dt>
              <dd>{cert.ttl}</dd>
            </div>
            <div>
              <dt className="font-medium text-muted-foreground">Issuing CA</dt>
              <dd className="truncate">{cert.issuingCA || "N/A"}</dd>
            </div>
            {cert.revokedAt && (
              <div>
                <dt className="font-medium text-muted-foreground">
                  Revoked At
                </dt>
                <dd>{formatDateTime(cert.revokedAt)}</dd>
              </div>
            )}
            <div>
              <dt className="font-medium text-muted-foreground">
                Renewal Attempts
              </dt>
              <dd>{cert.renewalAttempts}</dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <div className="flex gap-4">
        <Button
          variant="outline"
          onClick={() => {
            const blob = new Blob([cert.certificatePEM], {
              type: "application/x-pem-file",
            });
            const url = URL.createObjectURL(blob);
            const a = document.createElement("a");
            a.href = url;
            a.download = `${cert.commonName}.pem`;
            a.click();
            URL.revokeObjectURL(url);
          }}
        >
          <Download className="mr-2 h-4 w-4" />
          Download PEM
        </Button>
        {cert.status === "active" && (
          <Button variant="destructive" onClick={handleRevoke}>
            <Ban className="mr-2 h-4 w-4" />
            Revoke Certificate
          </Button>
        )}
      </div>
    </div>
  );
}
