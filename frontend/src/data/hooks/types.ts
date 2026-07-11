export interface HookFieldDoc {
  name: string;
  type?: string;
  description: string;
}

export interface HookEventDoc {
  name: string;
  aliases?: string[];
  description: string;
  matcher?: string;
  fields?: HookFieldDoc[];
  notes?: string[];
}

export interface HookSourceDoc {
  label: string;
  url: string;
}

export interface HookCliDoc {
  id: string;
  name: string;
  sources: HookSourceDoc[];
  commonFields?: HookFieldDoc[];
  notes?: string[];
  events: HookEventDoc[];
}

