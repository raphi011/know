import preview from "#.storybook/preview";
import { DocTree } from "@/components/doc-tree";
import { ToastProvider } from "@/components/ui/toast-provider";
import type { TreeNode, DocumentSummary } from "@/app/lib/knowhow/types";

const sampleTree: TreeNode[] = [
  {
    type: "folder",
    name: "guides",
    path: "guides",
    children: [
      {
        type: "document",
        name: "getting-started",
        path: "guides/getting-started.md",
      },
      { type: "document", name: "advanced", path: "guides/advanced.md" },
    ],
  },
  {
    type: "folder",
    name: "api",
    path: "api",
    children: [
      { type: "document", name: "endpoints", path: "api/endpoints.md" },
    ],
  },
  { type: "document", name: "README", path: "README.md" },
];

const sampleDocuments: DocumentSummary[] = [
  {
    id: "1",
    vaultId: "vault:test",
    path: "guides/getting-started.md",
    title: "Getting Started",
    labels: [],
    docType: null,
    createdAt: "2025-01-01T00:00:00Z",
    updatedAt: "2025-01-01T00:00:00Z",
  },
  {
    id: "2",
    vaultId: "vault:test",
    path: "guides/advanced.md",
    title: "Advanced",
    labels: [],
    docType: null,
    createdAt: "2025-01-01T00:00:00Z",
    updatedAt: "2025-01-01T00:00:00Z",
  },
  {
    id: "3",
    vaultId: "vault:test",
    path: "api/endpoints.md",
    title: "Endpoints",
    labels: [],
    docType: null,
    createdAt: "2025-01-01T00:00:00Z",
    updatedAt: "2025-01-01T00:00:00Z",
  },
  {
    id: "4",
    vaultId: "vault:test",
    path: "README.md",
    title: "README",
    labels: [],
    docType: null,
    createdAt: "2025-01-01T00:00:00Z",
    updatedAt: "2025-01-01T00:00:00Z",
  },
];

const meta = preview.meta({
  title: "Domain/DocTree",
  component: DocTree,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
});

export default meta;

export const WithContextMenu = meta.story({
  args: {
    tree: sampleTree,
    activePath: "guides/getting-started.md",
    vaultId: "vault:test",
    documents: sampleDocuments,
  },
  render: (args) => (
    <ToastProvider>
      <div className="h-96 w-64 border border-slate-200 dark:border-slate-800">
        <DocTree {...args} />
      </div>
    </ToastProvider>
  ),
});
