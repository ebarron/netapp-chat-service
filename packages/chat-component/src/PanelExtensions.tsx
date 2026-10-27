import {
  Component,
  createContext,
  type ComponentType,
  type ErrorInfo,
  type ReactNode,
  useContext,
} from 'react';

/** Opaque, data-only metadata carried by a validated built-in panel fallback. */
export interface PanelExtensionMetadata {
  kind: string;
  schemaVersion: number;
  data: unknown;
}

/**
 * The deliberately small capability set available to host extension renderers.
 * Extensions never receive the wire panel, fallback Markdown, or tool execution.
 */
export interface PanelExtensionContext {
  readOnly: boolean;
  sendMessage?: (message: string) => void;
}

export interface PanelExtensionRendererProps<T> {
  data: T;
  context: PanelExtensionContext;
}

/**
 * A build-time host registration. `normalize` is the trust boundary: return
 * normalized data or null/undefined to select the built-in fallback.
 */
export interface PanelExtensionDefinition<T> {
  kind: string;
  schemaVersion: number;
  normalize: (data: unknown) => T | null | undefined;
  render: ComponentType<PanelExtensionRendererProps<T>>;
}

type AnyPanelExtensionDefinition = PanelExtensionDefinition<any>;

export interface PanelExtensionRegistry {
  get(kind: string, schemaVersion: number): AnyPanelExtensionDefinition | undefined;
}

function extensionKey(kind: string, schemaVersion: number): string {
  return JSON.stringify([kind, schemaVersion]);
}

/** Creates an immutable registry and rejects ambiguous registrations eagerly. */
export function createPanelExtensionRegistry(
  definitions: readonly AnyPanelExtensionDefinition[],
): PanelExtensionRegistry {
  const entries = new Map<string, AnyPanelExtensionDefinition>();
  for (const definition of definitions) {
    if (!definition.kind.trim()) {
      throw new Error('Panel extension kind must be a non-empty string');
    }
    if (!Number.isSafeInteger(definition.schemaVersion) || definition.schemaVersion < 1) {
      throw new Error(
        `Panel extension "${definition.kind}" schemaVersion must be a positive safe integer`,
      );
    }
    if (typeof definition.normalize !== 'function') {
      throw new Error(
        `Panel extension "${definition.kind}" v${definition.schemaVersion} must define normalize`,
      );
    }
    const key = extensionKey(definition.kind, definition.schemaVersion);
    if (entries.has(key)) {
      throw new Error(
        `Duplicate panel extension registration: "${definition.kind}" v${definition.schemaVersion}`,
      );
    }
    entries.set(key, Object.freeze({ ...definition }));
  }
  return Object.freeze({
    get(kind: string, schemaVersion: number) {
      return entries.get(extensionKey(kind, schemaVersion));
    },
  });
}

const PanelExtensionRegistryContext = createContext<PanelExtensionRegistry | null>(null);

export interface ChatRenderExtensionProviderProps {
  registry: PanelExtensionRegistry;
  children: ReactNode;
}

/** Makes a host-owned, build-time registry available to chat panel renderers. */
export function ChatRenderExtensionProvider({
  registry,
  children,
}: ChatRenderExtensionProviderProps) {
  return (
    <PanelExtensionRegistryContext.Provider value={registry}>
      {children}
    </PanelExtensionRegistryContext.Provider>
  );
}

interface ExtensionErrorBoundaryProps {
  fallback: ReactNode;
  children: ReactNode;
}

interface ExtensionErrorBoundaryState {
  failed: boolean;
}

class ExtensionErrorBoundary extends Component<
  ExtensionErrorBoundaryProps,
  ExtensionErrorBoundaryState
> {
  state: ExtensionErrorBoundaryState = { failed: false };

  static getDerivedStateFromError(): ExtensionErrorBoundaryState {
    return { failed: true };
  }

  componentDidCatch(_error: Error, _info: ErrorInfo): void {
    // Deliberately isolated: one host extension must not break the dashboard.
  }

  render() {
    return this.state.failed ? this.props.fallback : this.props.children;
  }
}

interface PanelExtensionSlotProps {
  extension: PanelExtensionMetadata;
  fallback: ReactNode;
  readOnly?: boolean;
  onAction?: (message: string) => void;
}

/** @internal Resolves, normalizes, and isolates one extension render. */
export function PanelExtensionSlot({
  extension,
  fallback,
  readOnly,
  onAction,
}: PanelExtensionSlotProps) {
  const registry = useContext(PanelExtensionRegistryContext);
  const definition = registry?.get(extension.kind, extension.schemaVersion);
  if (!definition) return fallback;

  let normalized: unknown;
  try {
    normalized = definition.normalize(extension.data);
  } catch {
    return fallback;
  }
  if (normalized === null || normalized === undefined) return fallback;

  const Renderer = definition.render;
  const context: PanelExtensionContext = {
    readOnly: readOnly === true,
    sendMessage: onAction,
  };
  return (
    <ExtensionErrorBoundary fallback={fallback}>
      <Renderer data={normalized} context={context} />
    </ExtensionErrorBoundary>
  );
}
