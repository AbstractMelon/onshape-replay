import { parseOnshapeContext } from '$lib/onshape/context';

export const ssr = false;

export function load({ url }) {
  const context = parseOnshapeContext(url);
  return { context };
}
