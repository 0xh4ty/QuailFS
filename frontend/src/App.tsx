import { useEffect, useMemo, useState } from "react";
import type { Dataset, FileEntry, Node, View } from "./api/types";
import {
  generateRecoveryPhrase,
  unlock,
  configureBootstrapNodes,
  createDataset,
  listDatasets,
  listDirectory,
  getHomeDirectory,
  backup,
  restore,
  getDatasetFiles,
  listNodes,
} from "./api/client";

type AppStage = "splash" | "unlock" | "network-setup" | "app";

type BrowseMode = "local" | "network";

type BrowseEntry = FileEntry & {
  selected?: boolean;
};

type ActivityStatus = "running" | "success" | "error";
type ActivityKind = "backup" | "restore";

type Activity = {
  id: number;
  kind: ActivityKind;
  datasetName: string;
  status: ActivityStatus;
  time: string;
};

function App() {
  const [stage, setStage] = useState<AppStage>("splash");
  const [nextStage, setNextStage] = useState<"unlock" | "app">("unlock");
  const [splashLeaving, setSplashLeaving] = useState(false);

  const [bootstrapNodes, setBootstrapNodes] = useState<string[]>([]);

  const [view, setView] = useState<View>("dashboard");
  const [datasets, setDatasets] = useState<Dataset[]>([]);
  const [selectedDatasetId, setSelectedDatasetId] = useState("");

  const [creatingDataset, setCreatingDataset] = useState(false);
  const [datasetLabel, setDatasetLabel] = useState("");

  const [nodes, setNodes] = useState<Node[]>([]);
  const [networkConnected, setNetworkConnected] = useState(false);

  const [activities, setActivities] = useState<Activity[]>([]);
  const [nextActivityId, setNextActivityId] = useState(1);

  const [homeDirectory, setHomeDirectory] = useState("");
  const [browseMode, setBrowseMode] = useState<BrowseMode>("local");
  const [browsePath, setBrowsePath] = useState("");
  const [browseEntries, setBrowseEntries] = useState<BrowseEntry[]>([]);
  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(new Set());

  const MAX_ACTIVITIES = 20;

  const selectedDataset = useMemo(
    () => datasets.find((dataset) => dataset.id === selectedDatasetId),
    [datasets, selectedDatasetId],
  );

  useEffect(() => {
    if (stage !== "splash") {
      return;
    }

    const timer = window.setTimeout(() => {
      setSplashLeaving(true);

      window.setTimeout(() => {
        setStage(nextStage);
      }, 700);
    }, 3000);

    return () => {
      window.clearTimeout(timer);
    };
  }, [stage, nextStage]);

  useEffect(() => {
    if (view !== "local-browse" && view !== "network-browse") {
      return;
    }

    const mode: BrowseMode = view === "local-browse" ? "local" : "network";

    setBrowseMode(mode);
    setSelectedPaths(new Set());

    if (mode === "local") {
      setBrowsePath(homeDirectory);

      listDirectory(homeDirectory)
        .then(setBrowseEntries)
        .catch((error) => {
          console.error("Failed to list directory:", error);
          setBrowseEntries([]);
        });
    } else {
      setBrowsePath("/");

      if (!selectedDatasetId) {
        setBrowseEntries([]);
        return;
      }

      getDatasetFiles(selectedDatasetId)
        .then((entries) => setBrowseEntries(entries))
        .catch((error) => {
          console.error("Failed to load dataset files:", error);
          setBrowseEntries([]);
        });
    }
  }, [view, homeDirectory, selectedDatasetId]);

  const handleUnlock = async (phrase: string) => {
    const success = await unlock(phrase);

    if (success) {
      const home = await getHomeDirectory();
      setHomeDirectory(home);
      setStage("network-setup");
    }
  };

  const handleConfigureBootstrapNodes = async (
    addresses: string[],
  ): Promise<void> => {
    await configureBootstrapNodes(addresses);

    try {
      const fetchedDatasets = await listDatasets();

      setDatasets(fetchedDatasets);

      if (fetchedDatasets.length > 0) {
        setSelectedDatasetId(fetchedDatasets[0].id);
      }
    } catch (error) {
      console.error("Failed to load datasets:", error);
      setDatasets([]);
    }

    setBootstrapNodes(addresses);
    setNetworkConnected(true);
    setStage("app");
  };

  useEffect(() => {
    if (stage !== "app") {
      return;
    }

    listNodes()
      .then(setNodes)
      .catch((error) => {
        console.error("Failed to load network nodes:", error);
        setNodes([]);
      });
  }, [stage]);

  const handleSelectDataset = (datasetId: string) => {
    setSelectedDatasetId(datasetId);
    setView("dashboard");
  };

  const handleStartBackup = () => {
    setBrowseMode("local");
    setBrowsePath(homeDirectory);
    setSelectedPaths(new Set());
    setView("local-browse");
  };

  const handleBackup = async () => {
    const paths = Array.from(selectedPaths);

    if (paths.length === 0 || !selectedDatasetId) {
      return;
    }

    const dataset = datasets.find((item) => item.id === selectedDatasetId);

    if (!dataset) {
      return;
    }

    const activityId = nextActivityId;

    setNextActivityId((previous) => previous + 1);
    setActivities((previous) =>
      [
        {
          id: activityId,
          kind: "backup",
          datasetName: dataset.name,
          status: "running",
          time: "Now",
        } satisfies Activity,
        ...previous,
      ].slice(0, MAX_ACTIVITIES),
    );
    setView("dashboard");

    try {
      await backup(selectedDatasetId, paths);

      const completedAt = new Date().toLocaleTimeString([], {
        hour: "2-digit",
        minute: "2-digit",
      });

      setActivities((previous) =>
        previous.map((activity) =>
          activity.id === activityId
            ? { ...activity, status: "success", time: completedAt }
            : activity,
        ),
      );

      const refreshed = await listDatasets();
      setDatasets(refreshed);
    } catch (error) {
      console.error("Backup failed:", error);

      setActivities((previous) =>
        previous.map((activity) =>
          activity.id === activityId
            ? { ...activity, status: "error", time: "Failed" }
            : activity,
        ),
      );
    }
  };

  const handleRestore = async () => {
    const paths = Array.from(selectedPaths);

    if (paths.length === 0 || !selectedDatasetId) {
      return;
    }

    const dataset = datasets.find((item) => item.id === selectedDatasetId);

    if (!dataset) {
      return;
    }

    const activityId = nextActivityId;

    setNextActivityId((previous) => previous + 1);
    setActivities((previous) =>
      [
        {
          id: activityId,
          kind: "restore",
          datasetName: dataset.name,
          status: "running",
          time: "Now",
        } satisfies Activity,
        ...previous,
      ].slice(0, MAX_ACTIVITIES),
    );
    setView("dashboard");

    try {
      await restore(selectedDatasetId, paths, homeDirectory);

      const completedAt = new Date().toLocaleTimeString([], {
        hour: "2-digit",
        minute: "2-digit",
      });

      setActivities((previous) =>
        previous.map((activity) =>
          activity.id === activityId
            ? { ...activity, status: "success", time: completedAt }
            : activity,
        ),
      );
    } catch (error) {
      console.error("Restore failed:", error);

      setActivities((previous) =>
        previous.map((activity) =>
          activity.id === activityId
            ? { ...activity, status: "error", time: "Failed" }
            : activity,
        ),
      );
    }
  };

  const handleStartRestore = async () => {
    if (!selectedDatasetId) {
      return;
    }

    setBrowseMode("network");
    setBrowsePath("/");
    setBrowseEntries([]);
    setSelectedPaths(new Set());
    setView("network-browse");

    try {
      const entries = await getDatasetFiles(selectedDatasetId);
      setBrowseEntries(entries);
    } catch (error) {
      console.error("Failed to load dataset files:", error);
      setBrowseEntries([]);
    }
  };

  const handleBrowseBack = () => {
    setView("dashboard");
  };

  const handleDirectoryBack = async () => {
    if (browseMode === "network") {
      if (browsePath === "/") {
        return;
      }

      const segments = browsePath.split("/").filter(Boolean);
      segments.pop();

      const parent = segments.length === 0 ? "/" : `/${segments.join("/")}`;

      setBrowsePath(parent);

      try {
        const entries = await getDatasetFiles(selectedDatasetId);
        const prefix = parent === "/" ? "" : `${parent.replace(/\/$/, "")}/`;

        setBrowseEntries(
          entries.filter((item) => {
            const relative = item.path.startsWith(prefix)
              ? item.path.slice(prefix.length)
              : "";

            return relative.length > 0 && !relative.includes("/");
          }),
        );
      } catch (error) {
        console.error("Failed to list parent directory:", error);
        setBrowseEntries([]);
      }

      return;
    }

    if (browsePath === "/" || browsePath === homeDirectory) {
      return;
    }

    const segments = browsePath.split("/").filter(Boolean);
    segments.pop();

    const parent = segments.length === 0 ? "/" : `/${segments.join("/")}`;

    setBrowsePath(parent);

    try {
      const entries = await listDirectory(parent);
      setBrowseEntries(entries);
    } catch (error) {
      console.error("Failed to list parent directory:", error);
      setBrowseEntries([]);
    }
  };

  const handleOpenDirectory = async (entry: BrowseEntry) => {
    if (entry.type !== "directory") {
      return;
    }

    if (browseMode === "network") {
      if (!selectedDatasetId) {
        return;
      }

      setBrowsePath(entry.path);

      try {
        const entries = await getDatasetFiles(selectedDatasetId);
        const prefix = `${entry.path.replace(/\/$/, "")}/`;

        setBrowseEntries(
          entries.filter((item) => {
            if (!item.path.startsWith(prefix)) {
              return false;
            }

            const relative = item.path.slice(prefix.length);
            return relative.length > 0 && !relative.includes("/");
          }),
        );
      } catch (error) {
        console.error("Failed to open dataset directory:", error);
        setBrowseEntries([]);
      }

      return;
    }

    try {
      setBrowsePath(entry.path);
      const entries = await listDirectory(entry.path);
      setBrowseEntries(entries);
    } catch (error) {
      console.error("Failed to list directory:", error);
      setBrowseEntries([]);
    }
  };

  const handleToggleSelection = (entry: BrowseEntry) => {
    setSelectedPaths((previous) => {
      const next = new Set(previous);

      if (next.has(entry.path)) {
        next.delete(entry.path);
      } else {
        next.add(entry.path);
      }

      return next;
    });
  };

  const handleCreateDataset = () => {
    setDatasetLabel("");
    setCreatingDataset(true);
  };

  const handleSubmitCreateDataset = async () => {
    const label = datasetLabel.trim();

    if (!label) {
      return;
    }

    try {
      const dataset = await createDataset(label);

      setDatasets((previous) => [...previous, dataset]);
      setSelectedDatasetId(dataset.id);
      setView("dashboard");
      setCreatingDataset(false);
      setDatasetLabel("");
    } catch (error) {
      console.error("Failed to create dataset:", error);
    }
  };

  if (stage === "splash") {
    return (
      <div
        className={`startup-stage ${
          splashLeaving ? "startup-stage--leaving" : ""
        }`}
      >
        <SplashView />
      </div>
    );
  }

  if (stage === "unlock") {
    return (
      <div className="startup-stage startup-stage--revealed">
        <UnlockView onUnlock={handleUnlock} />
      </div>
    );
  }

  if (stage === "network-setup") {
    return (
      <div className="startup-stage startup-stage--revealed">
        <NetworkSetupView onContinue={handleConfigureBootstrapNodes} />
      </div>
    );
  }

  return (
    <>
      <AppShell
        view={view}
        datasets={datasets}
        nodes={nodes}
        selectedDatasetId={selectedDatasetId}
        selectedDataset={selectedDataset}
        browseMode={browseMode}
        homeDirectory={homeDirectory}
        browsePath={browsePath}
        browseEntries={browseEntries}
        selectedPaths={selectedPaths}
        onNavigate={setView}
        onSelectDataset={handleSelectDataset}
        onCreateDataset={handleCreateDataset}
        onStartBackup={handleStartBackup}
        onStartRestore={handleStartRestore}
        onBrowseBack={handleBrowseBack}
        onDirectoryBack={handleDirectoryBack}
        onOpenDirectory={handleOpenDirectory}
        onToggleSelection={handleToggleSelection}
        onBackup={handleBackup}
        activities={activities}
        networkConnected={networkConnected}
        onRestore={handleRestore}
      />

      {creatingDataset && (
        <div className="modal-backdrop">
          <div className="modal">
            <div className="modal-header">
              <h2>New dataset</h2>

              <button
                className="icon-button"
                type="button"
                onClick={() => setCreatingDataset(false)}
                aria-label="Close"
              >
                ×
              </button>
            </div>

            <div className="modal-body">
              <label className="field-label" htmlFor="dataset-label">
                Dataset name
              </label>

              <input
                id="dataset-label"
                className="text-input"
                type="text"
                value={datasetLabel}
                onChange={(event) => setDatasetLabel(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    void handleSubmitCreateDataset();
                  }
                }}
                placeholder="e.g. Personal"
                autoFocus
              />
            </div>

            <div className="modal-footer">
              <button
                className="secondary-button"
                type="button"
                onClick={() => setCreatingDataset(false)}
              >
                Cancel
              </button>

              <button
                className="primary-button"
                type="button"
                disabled={!datasetLabel.trim()}
                onClick={() => void handleSubmitCreateDataset()}
              >
                Create
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

function SplashView() {
  return (
    <main className="splash-view">
      <div className="splash-content">
        <div className="splash-logo" aria-hidden="true">
          <QuailLogo />
        </div>

        <h1 className="splash-title">QuailFS</h1>

        <p className="splash-tagline">
          A Distributed Encrypted Backup and Recovery System
        </p>
      </div>
    </main>
  );
}

function UnlockView({ onUnlock }: { onUnlock: (phrase: string) => void }) {
  const [phrase, setPhrase] = useState("");
  const [mode, setMode] = useState<"unlock" | "generate">("unlock");
  const [generatedPhrase, setGeneratedPhrase] = useState("");

  const canUnlock = phrase.trim().length > 0;

  const handleGenerate = async () => {
    try {
      const mnemonic = await generateRecoveryPhrase();

      setGeneratedPhrase(mnemonic);
      setMode("generate");
    } catch (error) {
      console.error("Failed to generate recovery phrase:", error);
    }
  };

  return (
    <main className="unlock-view">
      <div className="unlock-card">
        <div className="unlock-logo" aria-hidden="true">
          <QuailLogo />
        </div>

        <div className="unlock-panels">
          <div
            className={`unlock-panel-track ${
              mode === "generate" ? "unlock-panel-track--generate" : ""
            }`}
          >
            <section className="unlock-panel">
              <div className="unlock-heading">
                <h1>Welcome to QuailFS</h1>
                <p>Enter your recovery phrase to unlock your identity.</p>
              </div>

              <label className="unlock-field" htmlFor="recovery-phrase">
                <span>Recovery phrase</span>

                <textarea
                  id="recovery-phrase"
                  value={phrase}
                  onChange={(event) => setPhrase(event.target.value)}
                  placeholder="Enter your recovery phrase"
                  spellCheck={false}
                  autoComplete="off"
                  rows={4}
                />
              </label>

              <button
                className="primary-button unlock-button"
                type="button"
                disabled={!canUnlock}
                onClick={() => onUnlock(phrase)}
              >
                Unlock
              </button>

              <button
                className="text-button"
                type="button"
                onClick={handleGenerate}
              >
                Generate new recovery phrase
              </button>
            </section>

            <section className="unlock-panel">
              <div className="unlock-heading">
                <h1>Create your identity</h1>
                <p>Generate a new recovery phrase for your QuailFS identity.</p>
              </div>

              <div className="recovery-phrase-display">
                {generatedPhrase || "Your recovery phrase will appear here."}
              </div>

              <p className="recovery-warning">
                Write this phrase down and keep it somewhere safe. QuailFS
                cannot recover it for you.
              </p>

              <button
                className="primary-button unlock-button"
                type="button"
                onClick={() => onUnlock(generatedPhrase)}
              >
                I&apos;ve saved my phrase
              </button>

              <button
                className="text-button"
                type="button"
                onClick={() => setMode("unlock")}
              >
                ← Back
              </button>
            </section>
          </div>
        </div>
      </div>
    </main>
  );
}

function NetworkSetupView({
  onContinue,
}: {
  onContinue: (addresses: string[]) => Promise<void>;
}) {
  const [addresses, setAddresses] = useState<string[]>([""]);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState("");

  const updateAddress = (index: number, value: string) => {
    setAddresses((previous) =>
      previous.map((address, currentIndex) =>
        currentIndex === index ? value : address,
      ),
    );
  };

  const addAddress = () => {
    setAddresses((previous) => [...previous, ""]);
  };

  const removeAddress = (index: number) => {
    setAddresses((previous) =>
      previous.length === 1
        ? [""]
        : previous.filter((_, currentIndex) => currentIndex !== index),
    );
  };

  const handleContinue = async () => {
    const cleanedAddresses = addresses
      .map((address) => address.trim())
      .filter((address) => address.length > 0);

    if (cleanedAddresses.length === 0) {
      setError("Add at least one bootstrap node.");
      return;
    }

    setError("");
    setConnecting(true);

    try {
      await onContinue(cleanedAddresses);
    } catch (error) {
      console.error("Failed to configure bootstrap nodes:", error);
      setError(
        error instanceof Error
          ? error.message
          : "Failed to connect to bootstrap nodes.",
      );
    } finally {
      setConnecting(false);
    }
  };

  return (
    <main className="network-setup-view">
      <div className="network-setup-card">
        <div className="unlock-logo" aria-hidden="true">
          <QuailLogo />
        </div>

        <div className="network-setup-heading">
          <h1>Connect to the QuailFS network</h1>

          <p>
            Add one or more bootstrap nodes so QuailFS can discover the network
            and retrieve your catalog.
          </p>
        </div>

        <div className="bootstrap-node-list">
          {addresses.map((address, index) => (
            <div className="bootstrap-node-row" key={index}>
              <input
                className="text-input"
                type="text"
                value={address}
                onChange={(event) => updateAddress(index, event.target.value)}
                placeholder="/ip4/203.0.113.10/tcp/4001/p2p/12D3KooW..."
                spellCheck={false}
                autoComplete="off"
              />

              <button
                className="icon-button"
                type="button"
                onClick={() => removeAddress(index)}
                aria-label="Remove bootstrap node"
                disabled={addresses.length === 1}
              >
                ×
              </button>
            </div>
          ))}
        </div>

        <button
          className="text-button"
          type="button"
          onClick={addAddress}
          disabled={connecting}
        >
          + Add bootstrap node
        </button>

        {error && <p className="network-setup-error">{error}</p>}

        <button
          className="primary-button unlock-button"
          type="button"
          onClick={() => void handleContinue()}
          disabled={connecting}
        >
          {connecting ? "Connecting..." : "Continue"}
        </button>
      </div>
    </main>
  );
}

type AppShellProps = {
  view: View;
  datasets: Dataset[];
  nodes: Node[];
  selectedDatasetId: string;
  selectedDataset: Dataset | undefined;
  browseMode: BrowseMode;
  homeDirectory: string;
  browsePath: string;
  browseEntries: BrowseEntry[];
  selectedPaths: Set<string>;
  activities: Activity[];
  networkConnected: boolean;
  onNavigate: (view: View) => void;
  onSelectDataset: (datasetId: string) => void;
  onCreateDataset: () => void;
  onStartBackup: () => void;
  onStartRestore: () => void;
  onBrowseBack: () => void;
  onDirectoryBack: () => void;
  onOpenDirectory: (entry: BrowseEntry) => void;
  onToggleSelection: (entry: BrowseEntry) => void;
  onBackup: () => void;
  onRestore: () => void;
};

function AppShell({
  view,
  datasets,
  nodes,
  selectedDatasetId,
  selectedDataset,
  browseMode,
  homeDirectory,
  browsePath,
  browseEntries,
  selectedPaths,
  activities,
  networkConnected,
  onNavigate,
  onSelectDataset,
  onCreateDataset,
  onStartBackup,
  onStartRestore,
  onBrowseBack,
  onDirectoryBack,
  onOpenDirectory,
  onToggleSelection,
  onBackup,
  onRestore,
}: AppShellProps) {
  return (
    <div className="app-shell">
      <Sidebar
        view={view}
        datasets={datasets}
        selectedDatasetId={selectedDatasetId}
        activities={activities}
        networkConnected={networkConnected}
        onNavigate={onNavigate}
        onSelectDataset={onSelectDataset}
        onCreateDataset={onCreateDataset}
      />

      <main className="main-content">
        {view === "dashboard" && selectedDataset && (
          <DashboardView
            dataset={selectedDataset}
            onStartBackup={onStartBackup}
            onStartRestore={onStartRestore}
          />
        )}

        {view === "dataset" && selectedDataset && (
          <DatasetView dataset={selectedDataset} />
        )}

        {view === "local-browse" && (
          <BrowseFilesView
            mode="local"
            path={browsePath}
            homeDirectory={homeDirectory}
            entries={browseEntries}
            selectedPaths={selectedPaths}
            onBack={onBrowseBack}
            onDirectoryBack={onDirectoryBack}
            onOpenDirectory={onOpenDirectory}
            onToggleSelection={onToggleSelection}
            onBackup={onBackup}
            onRestore={onRestore}
          />
        )}

        {view === "network-browse" && (
          <BrowseFilesView
            mode="network"
            path={browsePath}
            homeDirectory={homeDirectory}
            entries={browseEntries}
            selectedPaths={selectedPaths}
            onBack={onBrowseBack}
            onDirectoryBack={onDirectoryBack}
            onOpenDirectory={onOpenDirectory}
            onToggleSelection={onToggleSelection}
            onBackup={onBackup}
            onRestore={onRestore}
          />
        )}

        {view === "nodes" && <NodesView nodes={nodes} />}
      </main>
    </div>
  );
}

type SidebarProps = {
  view: View;
  datasets: Dataset[];
  selectedDatasetId: string;
  activities: Activity[];
  networkConnected: boolean;
  onNavigate: (view: View) => void;
  onSelectDataset: (datasetId: string) => void;
  onCreateDataset: () => void;
};

function Sidebar({
  view,
  datasets,
  selectedDatasetId,
  activities,
  networkConnected,
  onNavigate,
  onSelectDataset,
  onCreateDataset,
}: SidebarProps) {
  return (
    <aside className="sidebar">
      <div className="sidebar-header">
        <button
          className="brand"
          type="button"
          onClick={() => onNavigate("dashboard")}
        >
          <span className="brand-mark" aria-hidden="true">
            <QuailLogo />
          </span>

          <span className="brand-name">QuailFS</span>
        </button>
      </div>

      <div className="sidebar-section">
        <div className="sidebar-section-header">
          <span>Datasets</span>

          <button
            className="icon-button"
            type="button"
            title="New dataset"
            onClick={onCreateDataset}
          >
            +
          </button>
        </div>

        <div className="dataset-list">
          {datasets.map((dataset) => (
            <button
              key={dataset.id}
              className={`dataset-item ${
                selectedDatasetId === dataset.id &&
                (view === "dashboard" || view === "dataset")
                  ? "dataset-item--active"
                  : ""
              }`}
              type="button"
              onClick={() => onSelectDataset(dataset.id)}
            >
              <span className="dataset-item-name">{dataset.name}</span>

              <span className="dataset-item-meta">{dataset.size}</span>
            </button>
          ))}
        </div>
      </div>

      <nav className="sidebar-navigation">
        <button
          className={`nav-item ${view === "nodes" ? "nav-item--active" : ""}`}
          type="button"
          onClick={() => onNavigate("nodes")}
        >
          <span className="nav-icon" aria-hidden="true">
            ◇
          </span>
          <span>Nodes</span>
        </button>
      </nav>

      <div className="sidebar-connection">
        <span
          className={`status-dot ${
            networkConnected ? "status-dot--online" : "status-dot--offline"
          }`}
        />
        <span>{networkConnected ? "Connected" : "Disconnected"}</span>
      </div>

      <div className="sidebar-activity">
        <div className="sidebar-divider" />

        {activities.map((activity) => (
          <div className="sidebar-activity-item" key={activity.id}>
            <div className="sidebar-activity-title">
              <span
                className={
                  activity.status === "running"
                    ? "sidebar-activity-spinner"
                    : activity.status === "success"
                      ? "sidebar-activity-success"
                      : "sidebar-activity-error"
                }
                aria-hidden="true"
              >
                {activity.status === "running"
                  ? "↻"
                  : activity.status === "success"
                    ? "✓"
                    : "×"}
              </span>
              <span>
                {activity.status === "running"
                  ? activity.kind === "backup"
                    ? "Backing up"
                    : "Restoring"
                  : activity.status === "success"
                    ? activity.kind === "backup"
                      ? "Backup successful"
                      : "Restore successful"
                    : activity.kind === "backup"
                      ? "Backup failed"
                      : "Restore failed"}
              </span>
            </div>
            <span className="sidebar-activity-dataset">
              {activity.datasetName}
            </span>
          </div>
        ))}
      </div>

      <div className="sidebar-footer">
        <div className="identity-indicator">
          <span className="status-dot status-dot--online" />
          <span>Identity unlocked</span>
        </div>
      </div>
    </aside>
  );
}

function DashboardView({
  dataset,
  onStartBackup,
  onStartRestore,
}: {
  dataset: Dataset;
  onStartBackup: () => void;
  onStartRestore: () => void;
}) {
  return (
    <div className="page dashboard-view">
      <PageHeader
        eyebrow="Dataset"
        title={dataset.name}
        description="Your encrypted backup dataset."
      />

      <section className="action-grid">
        <ActionCard
          title="Backup"
          description="Select local files and directories to back up."
          action="Start Backup"
          onClick={onStartBackup}
        />

        <ActionCard
          title="Restore"
          description="Browse your dataset and restore selected files."
          action="Restore"
          onClick={onStartRestore}
        />
      </section>

      <section className="stats-grid">
        <StatCard label="Files" value={dataset.files.toLocaleString()} />

        <StatCard label="Data" value={dataset.size} />

        <StatCard label="Last backup" value={dataset.lastBackup} />

        <StatCard label="Version" value={`v${dataset.generation}`} />
      </section>

      <section className="dashboard-section">
        <div className="section-heading">
          <div>
            <span className="section-eyebrow">Activity</span>
            <h2>Recent activity</h2>
          </div>
        </div>

        <div className="activity-list">
          <ActivityItem
            title="Last backup"
            description={`${dataset.files.toLocaleString()} files protected`}
            time={dataset.lastBackup}
          />
        </div>
      </section>
    </div>
  );
}

function DatasetView({ dataset }: { dataset: Dataset }) {
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);

    getDatasetFiles(dataset.id)
      .then((files) => {
        if (!cancelled) setEntries(files);
      })
      .catch((error) => {
        console.error("Failed to load dataset files:", error);
        if (!cancelled) setEntries([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [dataset.id]);

  return (
    <div className="page">
      <PageHeader
        eyebrow="Dataset"
        title={dataset.name}
        description="Browse the files represented by this dataset."
      />

      <div className="file-tree-card">
        <div className="file-tree-row file-tree-row--header">
          <span>Name</span>
          <span>Size</span>
        </div>

        {loading ? (
          <div className="empty-state">
            <span className="empty-state-title">Loading files…</span>
          </div>
        ) : entries.length === 0 ? (
          <div className="empty-state">
            <span className="empty-state-title">No files in this dataset</span>
          </div>
        ) : (
          entries.map((entry) => (
            <FileTreeRow
              key={entry.path}
              name={entry.name}
              type={entry.type}
              size={
                entry.type === "file" ? formatBytes(entry.size ?? 0) : undefined
              }
            />
          ))
        )}
      </div>
    </div>
  );
}

function BrowseFilesView({
  mode,
  path,
  homeDirectory,
  entries,
  selectedPaths,
  onBack,
  onDirectoryBack,
  onOpenDirectory,
  onToggleSelection,
  onBackup,
  onRestore,
}: {
  mode: BrowseMode;
  path: string;
  homeDirectory: string;
  entries: BrowseEntry[];
  selectedPaths: Set<string>;
  onBack: () => void;
  onDirectoryBack: () => void;
  onOpenDirectory: (entry: BrowseEntry) => void;
  onToggleSelection: (entry: BrowseEntry) => void;
  onBackup: () => void;
  onRestore: () => void;
}) {
  const isLocal = mode === "local";

  return (
    <div className="page browse-files-view">
      <div className="browse-header">
        <button className="back-button" type="button" onClick={onBack}>
          <span aria-hidden="true">←</span>
          <span>Back</span>
        </button>

        <div className="browse-title">
          <span className="section-eyebrow">
            {isLocal ? "Backup" : "Restore"}
          </span>

          <h1>
            {isLocal ? "Select files to back up" : "Select files to restore"}
          </h1>

          <p>
            {isLocal
              ? "Choose files and directories from this computer."
              : "Choose files and directories available in your dataset."}
          </p>
        </div>
      </div>

      <div className="browse-toolbar">
        <button
          className="directory-back-button"
          type="button"
          onClick={onDirectoryBack}
          disabled={path === "/" || path === homeDirectory}
        >
          <span aria-hidden="true">↑</span>
          <span>Parent</span>
        </button>

        <div className="browse-path">{path}</div>

        <div className="browse-selection-count">
          {selectedPaths.size} selected
        </div>
      </div>

      <div className="browse-files-card">
        <div className="browse-files-header">
          <span>Name</span>
          <span>Size</span>
        </div>

        {entries.length === 0 ? (
          <div className="empty-state">
            <span className="empty-state-title">This directory is empty</span>
          </div>
        ) : (
          entries.map((entry) => {
            const selected = selectedPaths.has(entry.path);

            return (
              <div
                key={entry.path}
                className={`browse-file-row ${
                  selected ? "browse-file-row--selected" : ""
                }`}
              >
                <div className="browse-file-main">
                  <button
                    className="browse-file-open"
                    type="button"
                    onClick={() => {
                      if (entry.type === "directory") {
                        onOpenDirectory(entry);
                      }
                    }}
                  >
                    <span
                      className={`file-icon ${
                        entry.type === "directory"
                          ? "file-icon--directory"
                          : "file-icon--file"
                      }`}
                      aria-hidden="true"
                    >
                      {entry.type === "directory" ? "□" : "▤"}
                    </span>

                    <span className="browse-file-name">{entry.name}</span>
                  </button>
                </div>

                <div className="browse-file-actions">
                  <span className="browse-file-size">
                    {entry.type === "directory"
                      ? "—"
                      : formatBytes(entry.size ?? 0)}
                  </span>

                  <button
                    className={`selection-checkbox ${
                      selected ? "selection-checkbox--selected" : ""
                    }`}
                    type="button"
                    aria-label={
                      selected
                        ? `Deselect ${entry.name}`
                        : `Select ${entry.name}`
                    }
                    onClick={() => onToggleSelection(entry)}
                  >
                    {selected ? "✓" : ""}
                  </button>
                </div>
              </div>
            );
          })
        )}
      </div>

      <div className="browse-footer">
        <span>
          {selectedPaths.size === 0
            ? "Select files or directories to continue."
            : `${selectedPaths.size} item${
                selectedPaths.size === 1 ? "" : "s"
              } selected.`}
        </span>

        <button
          className="primary-button"
          type="button"
          disabled={selectedPaths.size === 0}
          onClick={isLocal ? onBackup : onRestore}
        >
          {isLocal ? "Start Backup" : "Restore"}
        </button>
      </div>
    </div>
  );
}

function NodesView({ nodes }: { nodes: Node[] }) {
  const onlineCount = nodes.filter((node) => node.status === "Online").length;

  return (
    <div className="page">
      <PageHeader
        eyebrow="Network"
        title="Nodes"
        description="Bootstrap and dynamically discovered storage peers."
      />

      <section className="nodes-section">
        <div className="section-heading">
          <div>
            <span className="section-eyebrow">Bootstrap nodes</span>
            <h2>Configured entry points</h2>
          </div>
        </div>

        <div className="node-list">
          {nodes
            .filter((node) => node.bootstrap)
            .map((node) => (
              <NodeRow key={node.peerId} node={node} />
            ))}
        </div>
      </section>

      <section className="nodes-section">
        <div className="section-heading">
          <div>
            <span className="section-eyebrow">Known nodes</span>
            <h2>
              Network peers
              <span className="section-count">{onlineCount} online</span>
            </h2>
          </div>
        </div>

        <div className="node-list">
          {nodes.map((node) => (
            <NodeRow key={node.peerId} node={node} />
          ))}
        </div>
      </section>
    </div>
  );
}

function NodeRow({ node }: { node: Node }) {
  const statusClass =
    node.status === "Online"
      ? "node-status--online"
      : node.status === "Offline"
        ? "node-status--offline"
        : "node-status--expired";

  return (
    <div className="node-row">
      <div className="node-identity">
        <span
          className={`status-dot ${
            node.status === "Online"
              ? "status-dot--online"
              : "status-dot--offline"
          }`}
        />

        <div>
          <span className="node-peer-id">{node.peerId}</span>

          <span className="node-source">
            {node.bootstrap ? "Bootstrap" : "Discovered"}
          </span>
        </div>
      </div>

      <div className={`node-status ${statusClass}`}>{node.status}</div>

      <div className="node-latency">{node.latency}</div>
    </div>
  );
}

function PageHeader({
  eyebrow,
  title,
  description,
}: {
  eyebrow: string;
  title: string;
  description: string;
}) {
  return (
    <header className="page-header">
      <span className="page-eyebrow">{eyebrow}</span>
      <h1>{title}</h1>
      <p>{description}</p>
    </header>
  );
}

function ActionCard({
  title,
  description,
  action,
  onClick,
}: {
  title: string;
  description: string;
  action: string;
  onClick: () => void;
}) {
  return (
    <article className="action-card">
      <div className="action-card-content">
        <span className="action-card-label">{title}</span>

        <p>{description}</p>
      </div>

      <button className="primary-button" type="button" onClick={onClick}>
        {action}
      </button>
    </article>
  );
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <article className="stat-card">
      <span>{label}</span>
      <strong>{value}</strong>
    </article>
  );
}

function ActivityItem({
  title,
  description,
  time,
}: {
  title: string;
  description: string;
  time: string;
}) {
  return (
    <div className="activity-item">
      <span className="status-dot status-dot--online" />

      <div className="activity-content">
        <strong>{title}</strong>
        <span>{description}</span>
      </div>

      <time>{time}</time>
    </div>
  );
}

function FileTreeRow({
  name,
  type,
  size,
}: {
  name: string;
  type: "file" | "directory";
  size?: string;
}) {
  return (
    <div className="file-tree-row">
      <span className="file-tree-name">
        <span className="file-tree-icon" aria-hidden="true">
          {type === "directory" ? "□" : "▤"}
        </span>

        {name}
      </span>

      <span>{type === "directory" ? "—" : size}</span>
    </div>
  );
}

function formatBytes(bytes: number): string {
  if (bytes === 0) {
    return "0 B";
  }

  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.floor(Math.log(bytes) / Math.log(1024));

  const value = bytes / Math.pow(1024, index);

  return `${value.toFixed(value >= 10 || index === 0 ? 0 : 1)} ${units[index]}`;
}

function QuailLogo() {
  return (
    <svg
      viewBox="0 0 200 160"
      role="img"
      aria-label="Quail"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M53 116C32 105 24 83 30 62C36 41 56 27 80 29C102 31 120 44 127 63C134 82 128 105 111 119C95 132 72 130 53 116Z"
        fill="currentColor"
      />

      <path
        d="M115 59C128 43 145 36 163 40C148 48 140 60 139 75C136 69 128 63 115 59Z"
        fill="currentColor"
      />

      <path
        d="M158 40C169 35 181 38 188 46C177 47 168 51 161 57C165 50 164 45 158 40Z"
        fill="currentColor"
      />

      <circle cx="104" cy="57" r="5" fill="white" />

      <path
        d="M76 127C65 137 54 142 41 141"
        fill="none"
        stroke="currentColor"
        strokeWidth="7"
        strokeLinecap="round"
      />

      <path
        d="M96 127C91 139 84 146 74 151"
        fill="none"
        stroke="currentColor"
        strokeWidth="7"
        strokeLinecap="round"
      />

      <path
        d="M38 83C27 87 19 87 11 82C22 79 29 74 34 67"
        fill="none"
        stroke="currentColor"
        strokeWidth="6"
        strokeLinecap="round"
      />
    </svg>
  );
}

export default App;
