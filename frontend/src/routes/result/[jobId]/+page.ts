export const ssr = false;

export function load({ params }) {
  return { jobId: params.jobId };
}
