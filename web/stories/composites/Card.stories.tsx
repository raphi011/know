import preview from "#.storybook/preview";
import {
  Card,
  CardHeader,
  CardTitle,
  CardAction,
  CardContent,
  CardFooter,
} from "@/components/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

const meta = preview.meta({
  title: "Composites/Card",
  component: Card,
  tags: ["autodocs"],
  parameters: { layout: "centered" },
  decorators: [
    (Story) => (
      <div className="w-80">
        <Story />
      </div>
    ),
  ],
});

export default meta;

export const Default = meta.story({
  render: () => (
    <Card>
      <CardHeader>
        <CardTitle>Dashboard</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-sm text-slate-500 dark:text-slate-400">
          An overview of your recent activity and data.
        </p>
      </CardContent>
    </Card>
  ),
});

export const Interactive = meta.story({
  render: () => (
    <Card interactive>
      <CardHeader>
        <CardTitle>Jane Doe</CardTitle>
        <CardAction>
          <Badge variant="success">Active</Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        <p className="text-sm text-slate-500 dark:text-slate-400">
          12 posts · 340 views
        </p>
      </CardContent>
    </Card>
  ),
});

export const Highlighted = meta.story({
  render: () => (
    <Card highlighted>
      <CardHeader>
        <CardTitle>Featured</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-sm text-slate-500 dark:text-slate-400">
          This card is highlighted with the primary accent border.
        </p>
      </CardContent>
    </Card>
  ),
});

export const WithAllSubcomponents = meta.story({
  render: () => (
    <Card>
      <CardHeader>
        <CardTitle>Invitation</CardTitle>
        <CardAction>
          <Badge variant="warning">Pending</Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        <p className="text-sm text-slate-500 dark:text-slate-400">
          You have been invited to join the team. You have 7 days to respond.
        </p>
      </CardContent>
      <CardFooter>
        <Button variant="outline" size="sm">
          Decline
        </Button>
        <Button size="sm">Accept</Button>
      </CardFooter>
    </Card>
  ),
});
