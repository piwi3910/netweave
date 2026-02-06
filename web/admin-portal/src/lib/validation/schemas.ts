import { z } from "zod";

const DNS_NAME_REGEX = /^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*$/;
const IP_ADDRESS_REGEX = /^((25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(25[0-5]|2[0-4]\d|[01]?\d\d?)$/;

export const certificateSchema = z.object({
  commonName: z
    .string()
    .min(1, "Common Name is required")
    .max(64, "Common Name must be at most 64 characters")
    .regex(DNS_NAME_REGEX, "Invalid DNS name format"),
  altNames: z
    .string()
    .optional()
    .transform((val) =>
      val
        ? val
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean)
        : undefined
    )
    .pipe(
      z
        .array(z.string().regex(DNS_NAME_REGEX, "Invalid DNS name"))
        .optional()
    ),
  ipSANs: z
    .string()
    .optional()
    .transform((val) =>
      val
        ? val
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean)
        : undefined
    )
    .pipe(
      z
        .array(z.string().regex(IP_ADDRESS_REGEX, "Invalid IP address"))
        .optional()
    ),
  ttl: z.enum(["720h", "2160h", "4380h", "8760h"]),
});

export const tenantSchema = z.object({
  name: z
    .string()
    .min(1, "Name is required")
    .max(128, "Name must be at most 128 characters"),
  description: z
    .string()
    .max(512, "Description must be at most 512 characters")
    .optional()
    .default(""),
  contactEmail: z.string().email("Invalid email address"),
  metadata: z.record(z.string()).optional().default({}),
});

export const userSchema = z.object({
  subject: z
    .string()
    .min(1, "Certificate subject is required")
    .max(256, "Subject must be at most 256 characters"),
  commonName: z
    .string()
    .min(1, "Common Name is required")
    .max(64, "Common Name must be at most 64 characters"),
  email: z.string().email("Invalid email address").optional().or(z.literal("")),
  roleId: z.string().min(1, "Role is required"),
});

export type CertificateFormData = z.infer<typeof certificateSchema>;
export type TenantFormData = z.infer<typeof tenantSchema>;
export type UserFormData = z.infer<typeof userSchema>;
