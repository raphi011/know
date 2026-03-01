import preview from "#.storybook/preview";
import { Tabs } from "@/components/ui/tabs";

const meta = preview.meta({
  title: "UI/Tabs",
  component: Tabs,
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
    items: [
      {
        label: "Overview",
        content: (
          <p className="text-sm text-slate-600 dark:text-slate-400">
            Overview content goes here.
          </p>
        ),
      },
      {
        label: "Details",
        content: (
          <p className="text-sm text-slate-600 dark:text-slate-400">
            Detailed information and settings.
          </p>
        ),
      },
      {
        label: "Activity",
        content: (
          <p className="text-sm text-slate-600 dark:text-slate-400">
            Recent activity log.
          </p>
        ),
      },
    ],
  },
});

export const TwoTabs = meta.story({
  args: {
    items: [
      {
        label: "Posts",
        content: (
          <p className="text-sm text-slate-600 dark:text-slate-400">
            Your published posts.
          </p>
        ),
      },
      {
        label: "Drafts",
        content: (
          <p className="text-sm text-slate-600 dark:text-slate-400">
            Your unpublished drafts.
          </p>
        ),
      },
    ],
  },
});
