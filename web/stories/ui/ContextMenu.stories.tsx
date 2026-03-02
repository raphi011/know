import { useState } from "react";
import preview from "#.storybook/preview";
import {
  ContextMenu,
  ContextMenuItem,
  ContextMenuSeparator,
} from "@/components/ui/context-menu";
import {
  DocumentPlusIcon,
  FolderPlusIcon,
  PencilIcon,
  TrashIcon,
} from "@heroicons/react/24/outline";

const meta = preview.meta({
  title: "UI/ContextMenu",
  component: ContextMenu,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
});

export default meta;

function ContextMenuDemo() {
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState({ x: 0, y: 0 });

  return (
    <div
      className="flex h-screen items-center justify-center bg-slate-50 dark:bg-slate-950"
      onContextMenu={(e) => {
        e.preventDefault();
        setPosition({ x: e.clientX, y: e.clientY });
        setOpen(true);
      }}
    >
      <p className="text-sm text-slate-500 dark:text-slate-400">
        Right-click anywhere to open the context menu
      </p>

      <ContextMenu open={open} position={position} onClose={() => setOpen(false)}>
        <ContextMenuItem
          icon={<DocumentPlusIcon />}
          onClick={() => setOpen(false)}
        >
          New document
        </ContextMenuItem>
        <ContextMenuItem
          icon={<FolderPlusIcon />}
          onClick={() => setOpen(false)}
        >
          New folder
        </ContextMenuItem>
        <ContextMenuSeparator />
        <ContextMenuItem
          icon={<PencilIcon />}
          onClick={() => setOpen(false)}
        >
          Rename
        </ContextMenuItem>
        <ContextMenuItem
          icon={<TrashIcon />}
          destructive
          onClick={() => setOpen(false)}
        >
          Delete
        </ContextMenuItem>
      </ContextMenu>
    </div>
  );
}

export const Default = meta.story({
  render: () => <ContextMenuDemo />,
});
