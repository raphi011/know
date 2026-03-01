import preview from "#.storybook/preview";
import {
  DocumentTextIcon,
  MagnifyingGlassIcon,
  InboxIcon,
} from "@heroicons/react/24/outline";
import { EmptyState } from "@/components/empty-state";

const meta = preview.meta({
  title: "Composites/EmptyState",
  component: EmptyState,
  tags: ["autodocs"],
  parameters: { layout: "centered" },
  decorators: [
    (Story) => (
      <div className="w-96">
        <Story />
      </div>
    ),
  ],
});

export default meta;

export const Default = meta.story({
  args: {
    icon: <DocumentTextIcon />,
    title: "No posts yet",
    description: "Create your first post to get started.",
    action: { label: "Create post", onClick: () => {} },
  },
});

export const NoResults = meta.story({
  args: {
    icon: <MagnifyingGlassIcon />,
    title: "No results found",
    description: "Try adjusting your search or filter criteria.",
  },
});

export const EmptyInbox = meta.story({
  args: {
    icon: <InboxIcon />,
    title: "All caught up",
    description: "You have no new notifications.",
  },
});

export const WithoutDescription = meta.story({
  args: {
    icon: <DocumentTextIcon />,
    title: "No items",
  },
});

export const WithoutIcon = meta.story({
  args: {
    title: "Nothing here",
    description: "This section is empty.",
  },
});
