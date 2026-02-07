import NextAuth from "next-auth";
import Keycloak from "next-auth/providers/keycloak";

async function refreshAccessToken(token: Record<string, unknown>): Promise<Record<string, unknown>> {
  const issuer = process.env.AUTH_KEYCLOAK_ISSUER!;
  const tokenUrl = `${issuer}/protocol/openid-connect/token`;

  const response = await fetch(tokenUrl, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      grant_type: "refresh_token",
      client_id: process.env.AUTH_KEYCLOAK_ID!,
      client_secret: process.env.AUTH_KEYCLOAK_SECRET!,
      refresh_token: token.refreshToken as string,
    }),
  });

  if (!response.ok) {
    console.error("[auth] Token refresh failed:", response.status);
    return { ...token, error: "RefreshTokenError" };
  }

  const refreshed = await response.json();

  return {
    ...token,
    accessToken: refreshed.access_token,
    refreshToken: refreshed.refresh_token ?? token.refreshToken,
    expiresAt: Math.floor(Date.now() / 1000) + refreshed.expires_in,
  };
}

export const { handlers, signIn, signOut, auth } = NextAuth({
  providers: [
    Keycloak({
      clientId: process.env.AUTH_KEYCLOAK_ID!,
      clientSecret: process.env.AUTH_KEYCLOAK_SECRET!,
      issuer: process.env.AUTH_KEYCLOAK_ISSUER!,
    }),
  ],
  callbacks: {
    async jwt({ token, account, profile }) {
      if (account) {
        token.accessToken = account.access_token;
        token.refreshToken = account.refresh_token;
        token.expiresAt = account.expires_at;
      }
      if (profile) {
        token.roles = (profile as Record<string, unknown>).roles as
          | string[]
          | undefined;
      }

      // Refresh the access token if it has expired (with 60s buffer)
      const expiresAt = token.expiresAt as number | undefined;
      if (expiresAt && Date.now() / 1000 > expiresAt - 60) {
        return refreshAccessToken(token);
      }

      return token;
    },
    async session({ session, token }) {
      if (token.error === "RefreshTokenError") {
        return { ...session, accessToken: "", error: "RefreshTokenError" };
      }
      return {
        ...session,
        accessToken: token.accessToken as string,
        user: {
          ...session.user,
          roles: token.roles as string[] | undefined,
        },
      };
    },
  },
  pages: {
    signIn: "/login",
  },
});
