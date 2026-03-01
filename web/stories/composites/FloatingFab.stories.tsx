"use client";

import preview from "#.storybook/preview";
import { within, expect } from "storybook/test";
import { PlusIcon, BoltIcon } from "@heroicons/react/24/outline";
import { FloatingFab } from "@/components/floating-fab";
import { mobileViewport } from "../viewports";

const meta = preview.meta({
  title: "Composites/FloatingFab",
  component: FloatingFab,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    ...mobileViewport,
  },
});

export default meta;

function FloatingFabDefault() {
  return (
    <div className="relative h-[667px] bg-slate-50 dark:bg-slate-950">
      <div className="p-4">
        <p className="text-sm text-slate-500">Default FAB (primary-500)</p>
      </div>
      <FloatingFab icon={<PlusIcon />} label="Create" onClick={() => {}} />
    </div>
  );
}

export const Default = meta.story({
  render: () => <FloatingFabDefault />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    const fab = canvas.getByRole("button", { name: "Create" });
    await expect(fab).toBeEnabled();
  },
});

function FloatingFabActive() {
  return (
    <div className="relative h-[667px] bg-slate-50 dark:bg-slate-950">
      <div className="p-4">
        <p className="text-sm text-slate-500">
          Active variant FAB (accent-400)
        </p>
      </div>
      <FloatingFab
        icon={<BoltIcon />}
        label="Active action"
        onClick={() => {}}
        variant="active"
      />
    </div>
  );
}

export const ActiveVariant = meta.story({
  render: () => <FloatingFabActive />,
});

function FloatingFabDisabled() {
  return (
    <div className="relative h-[667px] bg-slate-50 dark:bg-slate-950">
      <div className="p-4">
        <p className="text-sm text-slate-500">Disabled FAB</p>
      </div>
      <FloatingFab
        icon={<PlusIcon />}
        label="Create"
        onClick={() => {}}
        disabled
      />
    </div>
  );
}

export const Disabled = meta.story({
  render: () => <FloatingFabDisabled />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    const fab = canvas.getByRole("button", { name: "Create" });
    await expect(fab).toBeDisabled();
  },
});
