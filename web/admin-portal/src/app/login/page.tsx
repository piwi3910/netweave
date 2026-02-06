"use client";

import { signIn } from "next-auth/react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default function LoginPage() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-muted/40">
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-primary text-primary-foreground font-bold text-lg">
            NW
          </div>
          <CardTitle className="text-2xl">NetWeave Admin</CardTitle>
          <CardDescription>
            Sign in to manage your O2-IMS infrastructure
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button
            className="w-full"
            size="lg"
            onClick={() => signIn("keycloak", { callbackUrl: "/dashboard" })}
          >
            Sign in with Keycloak
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
