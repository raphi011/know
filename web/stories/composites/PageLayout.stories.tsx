import preview from "#.storybook/preview";
import { PageLayout } from "@/components/page-layout";
import { Button } from "@/components/ui/button";
import { Card, CardHeader, CardContent } from "@/components/card";

const meta = preview.meta({
  title: "Composites/PageLayout",
  component: PageLayout,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="max-w-2xl">
        <Story />
      </div>
    ),
  ],
});

export default meta;

export const Default = meta.story({
  args: {
    title: "Dashboard",
    subtitle: "Overview of your recent activity",
    children: (
      <Card>
        <CardHeader>
          <h2 className="text-base font-semibold text-slate-900 dark:text-white">
            Recent Posts
          </h2>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-slate-500 dark:text-slate-400">
            Your latest content will appear here.
          </p>
        </CardContent>
      </Card>
    ),
  },
});

export const WithAction = meta.story({
  args: {
    title: "Posts",
    subtitle: "Manage your published content",
    action: <Button size="sm">Create post</Button>,
    children: (
      <Card>
        <CardContent>
          <p className="text-sm text-slate-500 dark:text-slate-400">
            Post list content here.
          </p>
        </CardContent>
      </Card>
    ),
  },
});

export const TitleOnly = meta.story({
  args: {
    title: "Settings",
    children: (
      <Card>
        <CardContent>
          <p className="text-sm text-slate-500 dark:text-slate-400">
            Settings form content.
          </p>
        </CardContent>
      </Card>
    ),
  },
});
