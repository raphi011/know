import preview from "#.storybook/preview";
import { Badge } from "@/components/ui/badge";

const meta = preview.meta({
  title: "UI/Badge",
  component: Badge,
  tags: ["autodocs"],
  parameters: { layout: "centered" },
  argTypes: {
    variant: {
      control: "select",
      options: ["success", "error", "warning", "highlight", "info", "subtle"],
    },
    size: { control: "select", options: ["sm", "default"] },
  },
});

export default meta;

export const Success = meta.story({
  args: { variant: "success", children: "Success" },
});

export const Error = meta.story({
  args: { variant: "error", children: "Error" },
});

export const Warning = meta.story({
  args: { variant: "warning", children: "Pending" },
});

export const Highlight = meta.story({
  args: { variant: "highlight", children: "#1" },
});

export const Info = meta.story({
  args: { variant: "info", children: "Info" },
});

export const Subtle = meta.story({
  args: { variant: "subtle", children: "Draft" },
});

export const Small = meta.story({
  args: { variant: "success", size: "sm", children: "S" },
});

export const AllVariants = meta.story({
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <Badge variant="success">Success</Badge>
      <Badge variant="error">Error</Badge>
      <Badge variant="warning">Pending</Badge>
      <Badge variant="highlight">#1</Badge>
      <Badge variant="info">Info</Badge>
      <Badge variant="subtle">Draft</Badge>
    </div>
  ),
});
