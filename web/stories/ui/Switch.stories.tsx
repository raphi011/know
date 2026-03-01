import preview from "#.storybook/preview";
import { Switch } from "@/components/ui/switch";

const meta = preview.meta({
  title: "UI/Switch",
  component: Switch,
  tags: ["autodocs"],
  parameters: { layout: "centered" },
});

export default meta;

export const Default = meta.story({
  args: { label: "Dark mode" },
});

export const Checked = meta.story({
  args: { label: "Notifications enabled", defaultChecked: true },
});

export const Disabled = meta.story({
  args: { label: "Disabled toggle", disabled: true },
});

export const DisabledChecked = meta.story({
  args: { label: "Locked on", disabled: true, defaultChecked: true },
});

export const WithoutLabel = meta.story({
  args: { "aria-label": "Toggle feature" },
});
