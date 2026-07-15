const ONSHAPE_DOC_PATH_RE = /\/documents\/([^/]+)\/w\/([^/]+)\/e\/([^/]+)/;

export interface OnshapeContext {
  documentId?: string;
  workspaceId?: string;
  elementId?: string;
  server: string;
}

function parseFromReferrer(server: string): {
  documentId?: string;
  workspaceId?: string;
  elementId?: string;
} {
  const ref = typeof document !== 'undefined' ? document.referrer : '';
  if (!ref) return {};

  try {
    const refUrl = new URL(ref);
    if (refUrl.origin !== server) return {};
    const match = refUrl.pathname.match(ONSHAPE_DOC_PATH_RE);
    if (match) {
      return {
        documentId: match[1],
        workspaceId: match[2],
        elementId: match[3]
      };
    }
  } catch {
    // Bad referrer URL, ignore
  }
  return {};
}

export function parseOnshapeContext(url: URL): OnshapeContext | null {
  const server = url.searchParams.get('server');

  if (!server) {
    return null;
  }

  const fromUrl = {
    documentId: url.searchParams.get('documentId') ?? undefined,
    workspaceId: url.searchParams.get('workspaceId') ?? undefined,
    elementId: url.searchParams.get('elementId') ?? undefined
  };

  // Use referrer to fill in any missing document context
  const fromReferrer = parseFromReferrer(server);

  return {
    documentId: fromUrl.documentId ?? fromReferrer.documentId,
    workspaceId: fromUrl.workspaceId ?? fromReferrer.workspaceId,
    elementId: fromUrl.elementId ?? fromReferrer.elementId,
    server
  };
}

export function hasDocumentContext(ctx: OnshapeContext): boolean {
  return !!(ctx.documentId && ctx.workspaceId && ctx.elementId);
}
