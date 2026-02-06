import apiClient from "./client";
import type {
  Certificate,
  CertificateRequest,
  MonitoringReport,
} from "@/types/api";

export async function listCertificates(): Promise<Certificate[]> {
  return apiClient.get<Certificate[]>("/certificates");
}

export async function getCertificate(
  serialNumber: string
): Promise<Certificate> {
  return apiClient.get<Certificate>(`/certificates/${serialNumber}`);
}

export async function issueCertificate(
  data: CertificateRequest
): Promise<Certificate> {
  return apiClient.post<Certificate>("/certificates", data);
}

export async function revokeCertificate(
  serialNumber: string
): Promise<void> {
  return apiClient.delete(`/certificates/${serialNumber}`);
}

export async function getMonitoringReport(): Promise<MonitoringReport> {
  return apiClient.get<MonitoringReport>("/certificates/monitoring");
}
