"use client";

import { useEffect, useState } from "react";
import {
  Building2,
  Users,
  FileKey,
  AlertTriangle,
  Activity,
  ShieldCheck,
} from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { CardSkeleton } from "@/components/shared/loading-skeleton";
import { listTenants } from "@/lib/api/tenants";
import { listUsers } from "@/lib/api/users";
import { getMonitoringReport } from "@/lib/api/certificates";
import { listAuditEvents } from "@/lib/api/audit";
import { getDisplayError } from "@/lib/utils/sanitize-error";
import type { MonitoringReport, AuditEvent, Tenant } from "@/types/api";
import { formatDateTime } from "@/lib/utils";

interface DashboardStats {
  tenantCount: number;
  userCount: number;
  certReport: MonitoringReport | null;
  recentEvents: AuditEvent[];
}

export default function DashboardPage() {
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function loadStats() {
      try {
        const [tenants, users, certReport, events] = await Promise.allSettled([
          listTenants(),
          listUsers(),
          getMonitoringReport(),
          listAuditEvents({ limit: 10 }),
        ]);

        setStats({
          tenantCount:
            tenants.status === "fulfilled" ? tenants.value.length : 0,
          userCount: users.status === "fulfilled" ? users.value.length : 0,
          certReport:
            certReport.status === "fulfilled" ? certReport.value : null,
          recentEvents:
            events.status === "fulfilled" ? events.value : [],
        });
      } catch (err) {
        setError(getDisplayError(err));
      }
    }
    loadStats();
  }, []);

  if (error) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4">
        <p className="text-sm text-destructive">{error}</p>
      </div>
    );
  }

  if (!stats) {
    return (
      <div className="space-y-6">
        <h1 className="text-3xl font-bold">Dashboard</h1>
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <CardSkeleton key={i} />
          ))}
        </div>
      </div>
    );
  }

  const certReport = stats.certReport;

  return (
    <div className="space-y-6">
      <h1 className="text-3xl font-bold">Dashboard</h1>

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <StatCard
          title="Tenants"
          value={stats.tenantCount}
          icon={Building2}
          description="Active tenants"
        />
        <StatCard
          title="Users"
          value={stats.userCount}
          icon={Users}
          description="Registered users"
        />
        <StatCard
          title="Certificates"
          value={certReport?.totalCertificates ?? 0}
          icon={FileKey}
          description={`${certReport?.activeCertificates ?? 0} active`}
        />
        <StatCard
          title="Expiring Soon"
          value={certReport?.expiringSoon ?? 0}
          icon={AlertTriangle}
          description="Within 30 days"
          variant={
            (certReport?.expiringSoon ?? 0) > 0 ? "warning" : "default"
          }
        />
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-lg">
              <ShieldCheck className="h-5 w-5" />
              Certificate Health
            </CardTitle>
          </CardHeader>
          <CardContent>
            {certReport ? (
              <div className="space-y-3">
                <StatusRow
                  label="Active"
                  value={certReport.activeCertificates}
                  variant="success"
                />
                <StatusRow
                  label="Expiring Soon"
                  value={certReport.expiringSoon}
                  variant={certReport.expiringSoon > 0 ? "warning" : "success"}
                />
                <StatusRow
                  label="Expired"
                  value={certReport.expired}
                  variant={certReport.expired > 0 ? "destructive" : "success"}
                />
                <StatusRow
                  label="Revoked"
                  value={certReport.revoked}
                  variant="secondary"
                />
                <StatusRow
                  label="Renewal Pending"
                  value={certReport.renewalsPending}
                  variant={
                    certReport.renewalsPending > 0 ? "warning" : "secondary"
                  }
                />
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                Certificate monitoring unavailable
              </p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-lg">
              <Activity className="h-5 w-5" />
              Recent Activity
            </CardTitle>
            <CardDescription>Latest audit events</CardDescription>
          </CardHeader>
          <CardContent>
            {stats.recentEvents.length > 0 ? (
              <div className="space-y-3">
                {stats.recentEvents.slice(0, 8).map((event) => (
                  <div
                    key={event.id}
                    className="flex items-center justify-between text-sm"
                  >
                    <div>
                      <span className="font-medium">{event.action}</span>
                      <span className="text-muted-foreground">
                        {" "}
                        {event.resourceType}
                      </span>
                    </div>
                    <span className="text-xs text-muted-foreground">
                      {formatDateTime(event.timestamp)}
                    </span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                No recent activity
              </p>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function StatCard({
  title,
  value,
  icon: Icon,
  description,
  variant = "default",
}: {
  title: string;
  value: number;
  icon: React.ElementType;
  description: string;
  variant?: "default" | "warning";
}) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        <Icon
          className={`h-4 w-4 ${
            variant === "warning"
              ? "text-yellow-600"
              : "text-muted-foreground"
          }`}
        />
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-bold">{value}</div>
        <p className="text-xs text-muted-foreground">{description}</p>
      </CardContent>
    </Card>
  );
}

function StatusRow({
  label,
  value,
  variant,
}: {
  label: string;
  value: number;
  variant: "success" | "warning" | "destructive" | "secondary";
}) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-sm">{label}</span>
      <Badge variant={variant}>{value}</Badge>
    </div>
  );
}
