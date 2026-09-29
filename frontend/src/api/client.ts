import type {
  BackupInfo,
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
  GetDatasetFiles,
  Restore,
  ListNodes,
  ListDatasets,
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
  const datasets = await ListDatasets();

  return datasets.map((dataset) => ({
    id: dataset.id,
    name: dataset.name,
    size: dataset.size,
    files: dataset.files,
    lastBackup: dataset.lastBackup,
    generation: dataset.generation,
  }));
}

export async function getDatasetFiles(datasetId: string): Promise<FileEntry[]> {
  const entries = await GetDatasetFiles(datasetId);

  return entries.map((entry) => ({
    name: entry.name,
    path: entry.path,
    type: entry.type as "file" | "directory",
    size: entry.size,
  }));
}

export async function createDataset(label: string): Promise<Dataset> {
  const dataset = await CreateDataset(label);

  return {
    id: dataset.id,
    name: dataset.name,
    size: dataset.size,
    files: dataset.files,
    lastBackup: dataset.lastBackup,
    generation: dataset.generation,
  };
}

export async function backup(
  datasetId: string,
  paths: string[],
): Promise<BackupInfo> {
  return Backup(datasetId, paths);
}

export async function restore(
  datasetId: string,
  paths: string[],
  destination: string,
): Promise<void> {
  await Restore(datasetId, paths, destination);
}

export async function listNodes(): Promise<Node[]> {
  const nodes = await ListNodes();

  return nodes.map((node) => ({
    peerId: node.peerId,
    status: node.status as "Online" | "Offline" | "Expired",
    latency: node.latency,
    bootstrap: node.bootstrap,
  }));
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
