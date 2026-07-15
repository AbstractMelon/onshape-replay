import type { ExportConfig } from './exportOptions';

export type JobStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';

export interface OutputFile {
  format: 'mp4' | 'gif' | 'zip';
  path: string;
  size: number;
}

export interface JobResponse {
  id: string;
  documentId: string;
  workspaceId: string;
  elementId: string;
  status: JobStatus;
  percentComplete: number;
  currentFeatureName: string;
  currentFeatureIndex: number;
  totalFeatures: number;
  estimatedRemainingSeconds: number;
  createdAt: string;
  startedAt: string | null;
  completedAt: string | null;
  outputs: OutputFile[];
  error?: string;
}

export interface ProgressSnapshot {
  jobId: string;
  status: JobStatus;
  currentFeatureIndex: number;
  currentFeatureName: string;
  totalFeatures: number;
  percentComplete: number;
  estimatedRemaining: number;
  errorMsg: string;
  outputs?: OutputFile[];
}

export interface FeatureManifestEntry {
  featureId: string;
  name: string;
  featureType: string;
  suppressed: boolean;
}

export interface JobManifest {
  jobId: string;
  documentId: string;
  workspaceId: string;
  elementId: string;
  exportConfig: ExportConfig;
  features: FeatureManifestEntry[];
  status: JobStatus;
  error: string;
  createdAt: string;
  startedAt: string;
  completedAt: string;
  outputs: OutputFile[];
}
