import { apiFetch, API_BASE } from './client';
import type { OnshapeContext } from '../onshape/context';
import type { ExportConfig } from '../types/exportOptions';
import type { JobResponse, JobManifest } from '../types/job';

export class AuthRequiredError extends Error {
  constructor() {
    super('Authentication required');
    this.name = 'AuthRequiredError';
  }
}

export async function checkAuthStatus(): Promise<boolean> {
  const res = await apiFetch('/auth/status');
  if (res.status === 200) {
    const body = await res.json();
    return body.status === 'authenticated';
  }
  return false;
}

export function redirectToLogin(currentUrl: string): void {
  const target = `${API_BASE}/auth/login?redirect=${encodeURIComponent(currentUrl)}`;
  window.location.href = target;
}

export async function getCurrentJob(
  ctx: OnshapeContext
): Promise<JobResponse | null> {
  const params = new URLSearchParams({
    documentId: ctx.documentId,
    workspaceId: ctx.workspaceId,
    elementId: ctx.elementId
  });
  const res = await apiFetch(`/jobs/current?${params.toString()}`);

  if (res.status === 404) return null;
  if (res.status === 401) throw new AuthRequiredError();
  if (!res.ok) throw new Error(`Failed to load current job (${res.status})`);
  return res.json();
}

export async function startJob(
  ctx: OnshapeContext,
  config: ExportConfig
): Promise<JobResponse> {
  const res = await apiFetch('/jobs', {
    method: 'POST',
    body: JSON.stringify({
      documentId: ctx.documentId,
      workspaceId: ctx.workspaceId,
      elementId: ctx.elementId,
      config
    })
  });

  if (res.status === 201) return res.json();
  if (res.status === 401) throw new AuthRequiredError();
  if (res.status === 400) {
    const body = await res.json();
    throw new Error(body.error ?? 'invalid request body');
  }
  throw new Error(`Failed to start job (${res.status})`);
}

export async function cancelJob(jobId: string): Promise<void> {
  const res = await apiFetch(`/jobs/${jobId}/cancel`, { method: 'POST' });

  if (res.status === 200) return;
  if (res.status === 401) throw new AuthRequiredError();
  if (res.status === 400) {
    console.warn('Cancel returned 400 (job already terminated):', await res.json());
    return;
  }
  throw new Error(`Failed to cancel job (${res.status})`);
}

export async function getJobManifest(
  jobId: string
): Promise<JobManifest | JobResponse> {
  const res = await apiFetch(`/jobs/${jobId}/manifest`);

  if (res.status === 200) return res.json();
  if (res.status === 401) throw new AuthRequiredError();
  if (res.status === 404) throw new Error('job not found');
  throw new Error(`Failed to load job manifest (${res.status})`);
}

export async function getJobById(jobId: string): Promise<JobResponse> {
  const res = await apiFetch(`/jobs/${jobId}`);

  if (res.status === 200) return res.json();
  if (res.status === 401) throw new AuthRequiredError();
  if (res.status === 404) throw new Error('job not found');
  throw new Error(`Failed to load job (${res.status})`);
}
