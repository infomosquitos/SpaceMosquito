export interface Cookie {
  name: string;
  value: string;
  domain: string;
  path: string;
  expires?: number;
  secure: boolean;
  httpOnly: boolean;
  sameSite?: string;
}

export interface Session {
  confluence_url: string;
  cookies: Cookie[];
}

export interface SessionStatus {
  valid: boolean;
  message: string;
  exists: boolean;
  username?: string;
}

export interface CrawlJob {
  id: string;
  space_url: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  progress: number;
  total_pages: number;
  completed: number;
  failed: number;
  error?: string;
  created_at: string;
  updated_at: string;
  started_at?: string;
  completed_at?: string;
}

export interface JobSnapshot {
  jobs: CrawlJob[];
  total: number;
  running: number;
  completed: number;
  failed: number;
  pending: number;
}

export interface CrawlSpace {
  space_key: string;
  space_name: string;
  space_url: string;
  pages_crawled: number;
  pages_total?: number;
  pages_storage?: number;
  pages_rendered?: number;
  last_crawled?: string;
  created_at?: string;
  /** Server path of the saved space page tree (e.g. /browse/KEY/); absent until generated. */
  browse_path?: string;
}

export interface CronSpaceConfig {
  space_key: string;
  space_url: string;
  full_crawl_enabled: boolean;
  full_crawl_interval: string;
  incr_crawl_enabled: boolean;
  incr_crawl_interval: string;
  detection: string;
}

export interface ExtensionSettings {
  backend_url: string;
  crawl_depth: 'all' | 'shallow';
  auto_renew?: boolean;
}

export interface SpaceInfo {
  spaceKey: string;
  spaceName: string;
  spaceURL: string;
  pageTitle: string;
  pageId?: number;
  host?: string;
  tabUrl?: string;
}
