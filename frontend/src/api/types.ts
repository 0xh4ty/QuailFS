export type View =
  "dashboard" | "dataset" | "local-browse" | "network-browse" | "nodes";

export type Dataset = {
  id: string;
  name: string;
  size: string;
  files: number;
  lastBackup: string;
  generation: number;
};

export type NodeStatus = "Online" | "Offline" | "Expired";

export type Node = {
  peerId: string;
  status: NodeStatus;
  latency: string;
  bootstrap: boolean;
};

export type BootstrapNode = {
  address: string;
};

export type UserInfo = {
  userId: string;
  publicKey: string;
};

export type FileEntry = {
  name: string;
  path: string;
  type: "file" | "directory";
  size?: number;
};

export type BackupStage =
  | "scanning"
  | "chunking"
  | "compressing"
  | "encrypting"
  | "encoding"
  | "storing"
  | "publishing"
  | "complete"
  | "error";

export type BackupProgress = {
  stage: BackupStage;
  progress: number;
  processedBytes: number;
  totalBytes: number;
};
