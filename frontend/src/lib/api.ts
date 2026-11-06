// Typed wrappers over the generated Wails bindings. Application code imports
// from here, never from wailsjs/ directly.

import * as AppService from '../../wailsjs/go/rpc/AppService';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import type { rpc } from '../../wailsjs/go/models';

export type AppInfo = rpc.AppInfo;

export const Topics = {
  fsChanged: 'fs.changed',
  workspaceChanged: 'workspace.changed',
  settingsChanged: 'settings.changed',
  recentChanged: 'recent.changed',
  indexReady: 'index.ready',
  indexUpdated: 'index.updated',
  notify: 'notify',
  markersUpdated: 'markers.updated',
} as const;

export type Topic = (typeof Topics)[keyof typeof Topics];

export interface Notification {
  id: string;
  level: 'info' | 'warn' | 'error';
  title: string;
  body?: string;
  file?: string;
  line?: number;
}

/** Subscribes to a backend topic; returns an unsubscribe function. */
export function on<T = unknown>(topic: Topic, fn: (payload: T) => void): () => void {
  return EventsOn(topic, (payload: T) => fn(payload));
}

export const app = {
  info: (): Promise<AppInfo> => AppService.Info(),
};
