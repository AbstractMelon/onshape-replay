import type { OnshapeContext } from './context';

export function sendApplicationInit(ctx: OnshapeContext): void {
  if (!ctx.server) return;

  window.parent.postMessage(
    {
      documentId: ctx.documentId,
      workspaceId: ctx.workspaceId,
      elementId: ctx.elementId,
      messageName: 'applicationInit'
    },
    ctx.server
  );
}

export function sendMessageBubble(ctx: OnshapeContext, message: string): void {
  if (!ctx.server) return;

  window.parent.postMessage(
    {
      documentId: ctx.documentId,
      workspaceId: ctx.workspaceId,
      elementId: ctx.elementId,
      messageName: 'showMessageBubble',
      message
    },
    ctx.server
  );
}

export function onOnshapeMessage(
  ctx: OnshapeContext,
  handler: (data: unknown) => void
): () => void {
  const listener = (event: MessageEvent) => {
    if (event.origin !== ctx.server) return;
    handler(event.data);
  };

  window.addEventListener('message', listener);
  return () => window.removeEventListener('message', listener);
}
