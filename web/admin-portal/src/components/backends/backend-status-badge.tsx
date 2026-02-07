"use client";

import { Badge } from "@/components/ui/badge";

interface BackendStatusBadgeProps {
  status: string;
}

export function BackendStatusBadge({ status }: BackendStatusBadgeProps) {
  const variant = getStatusVariant(status);
  return <Badge variant={variant}>{status}</Badge>;
}

function getStatusVariant(
  status: string
): "success" | "destructive" | "secondary" {
  switch (status.toLowerCase()) {
    case "healthy":
      return "success";
    case "unhealthy":
      return "destructive";
    case "unknown":
    default:
      return "secondary";
  }
}
