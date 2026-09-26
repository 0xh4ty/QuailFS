import type {
  BackupProgress,
  Dataset,
  FileEntry,
  Node,
  UserInfo,
} from "./types";

import {
  GenerateRecoveryPhrase,
  Unlock,
  ConfigureBootstrapNodes,
  CreateDataset,
  ListDirectory,
  GetHomeDirectory,
  Backup,
} from "../../wailsjs/go/main/App";

export async function unlock(recoveryPhrase: string): Promise<boolean> {
  return Unlock(recoveryPhrase);
}

export async function configureBootstrapNodes(
  addresses: string[],
): Promise<void> {
  await ConfigureBootstrapNodes(addresses);
}

export async function getUserInfo(): Promise<UserInfo> {
  throw new Error("Wails getUserInfo method has not been connected yet.");
}

export async function listDatasets(): Promise<Dataset[]> {
  throw new Error("Wails listDatasets method has not been connected yet.");
}

export async function getDatasetFiles(datasetId: string): Promise<FileEntry[]> {
  void datasetId;

  throw new Error("Wails getDatasetFiles method has not been connected yet.");
}

export async function createDataset(label: string): Promise<Dataset> {
  const dataset = await CreateDataset(label);

  return {
    id: dataset.ID,
    name: dataset.Name,
    size: dataset.Size,
    files: dataset.Files,
    lastBackup: dataset.LastBackup,
  };
}

export async function backup(
  datasetId: string,
  paths: string[],
): Promise<void> {
  await Backup(datasetId, paths);
}

export async function restore(
  datasetId: string,
  paths: string[],
  destination: string,
): Promise<void> {
  void datasetId;
  void paths;
  void destination;

  throw new Error("Wails restore method has not been connected yet.");
}

export async function listNodes(): Promise<Node[]> {
  throw new Error("Wails listNodes method has not been connected yet.");
}

export async function addBootstrapNode(
  peerId: string,
  address: string,
): Promise<void> {
  void peerId;
  void address;

  throw new Error("Wails addBootstrapNode method has not been connected yet.");
}

export async function removeBootstrapNode(peerId: string): Promise<void> {
  void peerId;

  throw new Error(
    "Wails removeBootstrapNode method has not been connected yet.",
  );
}

export async function getBackupProgress(): Promise<BackupProgress> {
  throw new Error("Wails getBackupProgress method has not been connected yet.");
}

export async function generateRecoveryPhrase(): Promise<string> {
  return GenerateRecoveryPhrase();
}

export async function listDirectory(path: string): Promise<FileEntry[]> {
  const entries = await ListDirectory(path);

  return entries.map((entry) => ({
    name: entry.name,
    path: entry.path,
    type: entry.type as "file" | "directory",
    size: entry.size,
  }));
}

export async function getHomeDirectory(): Promise<string> {
  return GetHomeDirectory();
}
