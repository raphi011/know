"use client";

import {
  Tab,
  TabGroup,
  TabList,
  TabPanel,
  TabPanels,
  type TabGroupProps,
} from "@headlessui/react";
import { cn } from "@/lib/utils";

type TabItem = {
  label: string;
  content: React.ReactNode;
};

type TabsProps = Omit<TabGroupProps, "children"> & {
  items: TabItem[];
  className?: string;
};

function Tabs({ items, className, ...props }: TabsProps) {
  return (
    <TabGroup {...props}>
      <TabList
        className={cn(
          "flex gap-1 rounded-xl bg-slate-100 p-1 dark:bg-slate-800",
          className,
        )}
      >
        {items.map((item) => (
          <Tab
            key={item.label}
            className={cn(
              "flex-1 rounded-lg px-3 py-2 text-sm font-medium",
              "text-slate-600 transition-colors duration-150",
              "cursor-pointer hover:bg-slate-200 hover:text-slate-900 dark:hover:bg-slate-700 dark:hover:text-slate-200",
              "focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500",
              "data-[selected]:bg-white data-[selected]:text-primary-600 data-[selected]:shadow-sm",
              "dark:text-slate-400",
              "dark:data-[selected]:bg-slate-900 dark:data-[selected]:text-primary-400",
            )}
          >
            {item.label}
          </Tab>
        ))}
      </TabList>
      <TabPanels className="mt-3">
        {items.map((item) => (
          <TabPanel key={item.label}>{item.content}</TabPanel>
        ))}
      </TabPanels>
    </TabGroup>
  );
}

export { Tabs };
export type { TabsProps, TabItem };
