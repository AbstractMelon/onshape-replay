import { writable } from 'svelte/store';
import type { JobResponse, ProgressSnapshot } from '../types/job';

export type ConnectionState = 'connected' | 'reconnecting' | 'closed';

export interface JobStoreState {
  job: JobResponse | null;
  connectionState: ConnectionState;
}

function createJobStore() {
  const { subscribe, update, set } = writable<JobStoreState>({
    job: null,
    connectionState: 'closed'
  });

  return {
    subscribe,
    setJob(job: JobResponse | null) {
      set({ job, connectionState: 'closed' });
    },
    applySnapshot(snapshot: ProgressSnapshot) {
      update((state) => ({
        connectionState: 'connected' as ConnectionState,
        job: state.job
          ? {
              ...state.job,
              status: snapshot.status,
              percentComplete: snapshot.percentComplete,
              currentFeatureName: snapshot.currentFeatureName,
              currentFeatureIndex: snapshot.currentFeatureIndex,
              totalFeatures: snapshot.totalFeatures,
              estimatedRemainingSeconds: Math.round(snapshot.estimatedRemaining / 1e9),
              error: snapshot.errorMsg || state.job.error,
              outputs: snapshot.outputs ?? state.job.outputs
            }
          : null
      }));
    },
    setConnectionState(connectionState: ConnectionState) {
      update((state) => ({ ...state, connectionState }));
    }
  };
}

export const jobStore = createJobStore();
