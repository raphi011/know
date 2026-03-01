import { Badge } from "@/components/ui/badge";

function ColorSwatch({
  name,
  token,
  hex,
}: {
  name: string;
  token: string;
  hex: string;
}) {
  return (
    <div className="flex items-center gap-3">
      <div
        className="size-10 rounded-lg ring-1 ring-slate-200"
        style={{ backgroundColor: hex }}
      />
      <div>
        <div className="text-sm font-medium text-slate-900 dark:text-white">
          {name}
        </div>
        <div className="text-xs text-slate-500 dark:text-slate-400">
          {token} · {hex}
        </div>
      </div>
    </div>
  );
}

function ColorTokens() {
  return (
    <div className="space-y-8 p-6 font-sans">
      <div>
        <h2 className="mb-4 text-xl font-semibold tracking-tight text-slate-900 dark:text-white">
          Primary — Blue/Indigo
        </h2>
        <div className="grid grid-cols-2 gap-3">
          <ColorSwatch name="primary-50" token="bg-primary-50" hex="#EEF2FF" />
          <ColorSwatch
            name="primary-100"
            token="bg-primary-100"
            hex="#E0E7FF"
          />
          <ColorSwatch
            name="primary-200"
            token="bg-primary-200"
            hex="#C7D2FE"
          />
          <ColorSwatch
            name="primary-300"
            token="bg-primary-300"
            hex="#A5B4FC"
          />
          <ColorSwatch
            name="primary-400"
            token="bg-primary-400"
            hex="#818CF8"
          />
          <ColorSwatch
            name="primary-500"
            token="bg-primary-500"
            hex="#6366F1"
          />
          <ColorSwatch
            name="primary-600"
            token="bg-primary-600"
            hex="#4F46E5"
          />
          <ColorSwatch
            name="primary-700"
            token="bg-primary-700"
            hex="#4338CA"
          />
          <ColorSwatch
            name="primary-800"
            token="bg-primary-800"
            hex="#3730A3"
          />
          <ColorSwatch
            name="primary-900"
            token="bg-primary-900"
            hex="#312E81"
          />
          <ColorSwatch
            name="primary-950"
            token="bg-primary-950"
            hex="#1E1B4E"
          />
        </div>
      </div>

      <div>
        <h2 className="mb-4 text-xl font-semibold tracking-tight text-slate-900 dark:text-white">
          Accent — Amber
        </h2>
        <div className="grid grid-cols-2 gap-3">
          <ColorSwatch name="accent-50" token="bg-accent-50" hex="#FFFBEB" />
          <ColorSwatch name="accent-100" token="bg-accent-100" hex="#FEF3C7" />
          <ColorSwatch name="accent-200" token="bg-accent-200" hex="#FDE68A" />
          <ColorSwatch name="accent-400" token="bg-accent-400" hex="#E5A820" />
          <ColorSwatch name="accent-500" token="bg-accent-500" hex="#A8680B" />
          <ColorSwatch name="accent-600" token="bg-accent-600" hex="#8C5709" />
        </div>
      </div>

      <div>
        <h2 className="mb-4 text-xl font-semibold tracking-tight text-slate-900 dark:text-white">
          Error — Warm Red
        </h2>
        <div className="grid grid-cols-2 gap-3">
          <ColorSwatch name="red-50" token="bg-red-50" hex="#FEF3F1" />
          <ColorSwatch name="red-100" token="bg-red-100" hex="#FDE5DF" />
          <ColorSwatch name="red-500" token="bg-red-500" hex="#E45A42" />
          <ColorSwatch name="red-600" token="bg-red-600" hex="#CA4129" />
          <ColorSwatch name="red-950" token="bg-red-950" hex="#40130C" />
        </div>
      </div>

      <div>
        <h2 className="mb-4 text-xl font-semibold tracking-tight text-slate-900 dark:text-white">
          Badge Variants
        </h2>
        <div className="flex flex-wrap gap-3">
          <Badge variant="success">Success</Badge>
          <Badge variant="error">Error</Badge>
          <Badge variant="warning">Warning</Badge>
          <Badge variant="highlight">Highlight</Badge>
          <Badge variant="info">Info</Badge>
          <Badge variant="subtle">Subtle</Badge>
        </div>
      </div>
    </div>
  );
}

const meta = {
  title: "Design System/Color Tokens",
  component: ColorTokens,
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;

export const Default = {};
