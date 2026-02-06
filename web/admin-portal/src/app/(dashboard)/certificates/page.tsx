"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { FileKey, Plus, Ban, Download, AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { EmptyState } from "@/components/shared/empty-state";
import { listCertificates, revokeCertificate } from "@/lib/api/certificates";
import { getDisplayError } from "@/lib/utils/sanitize-error";
import type { Certificate } from "@/types/api";
import { formatDate, daysUntil } from "@/lib/utils";

export default function CertificatesPage() {
  const router = useRouter();
  const [certs, setCerts] = useState<Certificate[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadCerts();
  }, []);

  async function loadCerts() {
    try {
      setLoading(true);
      const data = await listCertificates();
      setCerts(data);
    } catch (err) {
      setError(getDisplayError(err));
    } finally {
      setLoading(false);
    }
  }

  async function handleRevoke(serial: string, cn: string) {
    if (
      !confirm(
        `Revoke certificate "${cn}" (${serial})? This action cannot be undone.`
      )
    ) {
      return;
    }
    try {
      await revokeCertificate(serial);
      setCerts((prev) =>
        prev.map((c) =>
          c.serialNumber === serial ? { ...c, status: "revoked" } : c
        )
      );
    } catch (err) {
      setError(getDisplayError(err));
    }
  }

  const filtered = certs.filter(
    (c) =>
      c.commonName.toLowerCase().includes(search.toLowerCase()) ||
      c.serialNumber.toLowerCase().includes(search.toLowerCase())
  );

  const statusVariant = (status: string) => {
    switch (status) {
      case "active":
        return "success" as const;
      case "expiring_soon":
        return "warning" as const;
      case "expired":
      case "revoked":
        return "destructive" as const;
      case "renewal_pending":
        return "warning" as const;
      default:
        return "secondary" as const;
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold">Certificates</h1>
        <Button onClick={() => router.push("/certificates/new")}>
          <Plus className="mr-2 h-4 w-4" />
          Issue Certificate
        </Button>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      <Input
        placeholder="Search by CN or serial number..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="max-w-sm"
      />

      {loading ? (
        <TableSkeleton />
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={FileKey}
          title="No certificates found"
          description={
            search
              ? "No certificates match your search"
              : "Issue your first certificate"
          }
          actionLabel={!search ? "Issue Certificate" : undefined}
          onAction={
            !search ? () => router.push("/certificates/new") : undefined
          }
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Common Name</TableHead>
              <TableHead>Serial Number</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Expires</TableHead>
              <TableHead>Issued</TableHead>
              <TableHead className="w-28">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.map((cert) => {
              const days = daysUntil(cert.expiresAt);
              return (
                <TableRow key={cert.serialNumber}>
                  <TableCell>
                    <Link
                      href={`/certificates/${cert.serialNumber}`}
                      className="font-medium hover:underline"
                    >
                      {cert.commonName}
                    </Link>
                  </TableCell>
                  <TableCell className="text-sm font-mono text-muted-foreground">
                    {cert.serialNumber.substring(0, 16)}...
                  </TableCell>
                  <TableCell>
                    <Badge variant={statusVariant(cert.status)}>
                      {cert.status.replace("_", " ")}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1 text-sm">
                      {days <= 30 && days > 0 && cert.status === "active" && (
                        <AlertTriangle className="h-3 w-3 text-yellow-600" />
                      )}
                      {formatDate(cert.expiresAt)}
                      {days > 0 && (
                        <span className="text-xs text-muted-foreground">
                          ({days}d)
                        </span>
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(cert.issuedAt)}
                  </TableCell>
                  <TableCell>
                    <div className="flex gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
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
                        aria-label="Download certificate"
                      >
                        <Download className="h-4 w-4" />
                      </Button>
                      {cert.status === "active" && (
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() =>
                            handleRevoke(cert.serialNumber, cert.commonName)
                          }
                          aria-label="Revoke certificate"
                        >
                          <Ban className="h-4 w-4 text-destructive" />
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
