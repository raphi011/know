import preview from "#.storybook/preview";
import { DocTree } from "@/components/doc-tree";
import type { TreeNode } from "@/app/lib/knowhow/types";

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
  },
  render: (args) => (
    <div className="h-96 w-64 border border-slate-200 dark:border-slate-800">
      <DocTree {...args} />
    </div>
  ),
});
