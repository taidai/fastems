import type { ReactNode } from "react"

export type IconName =
  | "solar" | "battery" | "home" | "bolt" | "plug" | "bars"
  | "bell" | "flow" | "devices" | "sliders" | "report" | "gear" | "sun" | "map"
  | "close" | "send" | "sparkle" | "coin" | "yuan" | "efficiency" | "factory"
  | "cube" | "tag" | "expand" | "user" | "layers"

const paths: Record<IconName, ReactNode> = {
  solar: <>
    <rect x="3" y="4" width="18" height="12" rx="1.6" />
    <path d="M3 8h18M3 12h18M9 4v12M15 4v12M12 16v3M8.5 19h7" />
  </>,
  battery: <>
    <rect x="3" y="7" width="15" height="10" rx="2.2" />
    <path d="M21 10.2v3.6M7 10.5v3M10.5 10.5v3M14 10.5v3" />
  </>,
  home: <>
    <path d="M4 11.2 12 4l8 7.2" />
    <path d="M6.2 10v9.2h11.6V10" />
    <path d="M10 19.2v-5h4v5" />
  </>,
  bolt: <path d="M13.2 2.5 5.4 13.6h4.9l-1 7.9 7.9-11.3h-5l1-7.7Z" />,
  plug: <>
    <path d="M9 3.2v4.6M15 3.2v4.6" />
    <path d="M6.8 7.8h10.4v3.1a5.2 5.2 0 0 1-10.4 0V7.8Z" />
    <path d="M12 16.1v4.7" />
  </>,
  bars: <path d="M5 20V10M12 20V4M19 20v-6.5" />,
  bell: <>
    <path d="M18.4 16.3H5.6l1.5-2.2v-3.7a4.9 4.9 0 0 1 9.8 0v3.7l1.5 2.2Z" />
    <path d="M10 19.2a2.1 2.1 0 0 0 4 0" />
  </>,
  flow: <>
    <path d="M4 6.5h6.5M4 12h9.5M4 17.5h6.5" />
    <circle cx="18" cy="6.5" r="2.1" />
    <circle cx="18" cy="17.5" r="2.1" />
    <path d="M18 8.6v6.8" />
  </>,
  devices: <>
    <rect x="3.2" y="3.2" width="7.4" height="7.4" rx="1.6" />
    <rect x="13.4" y="3.2" width="7.4" height="7.4" rx="1.6" />
    <rect x="3.2" y="13.4" width="7.4" height="7.4" rx="1.6" />
    <rect x="13.4" y="13.4" width="7.4" height="7.4" rx="1.6" />
  </>,
  sliders: <>
    <path d="M4 7h9.2M18.6 7H20M4 17h3.4M11.6 17H20" />
    <circle cx="15.8" cy="7" r="2.1" />
    <circle cx="8.8" cy="17" r="2.1" />
  </>,
  report: <>
    <path d="M4 20h16" />
    <path d="M6 20v-7M11 20V8M16 20v-4.5M20.5 20V5" />
  </>,
  gear: <>
    <circle cx="12" cy="12" r="3.1" />
    <path d="M12 2.8v2.1M12 19.1v2.1M4.9 4.9l1.5 1.5M17.6 17.6l1.5 1.5M2.8 12h2.1M19.1 12h2.1M4.9 19.1l1.5-1.5M17.6 6.4l1.5-1.5" />
  </>,
  sun: <>
    <circle cx="12" cy="12" r="4" />
    <path d="M12 2.5v2M12 19.5v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M2.5 12h2M19.5 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4" />
  </>,
  map: <>
    <path d="M9 4 3 6v14l6-2 6 2 6-2V4l-6 2-6-2Z" />
    <path d="M9 4v14M15 6v14" />
  </>,
  close: <path d="m6 6 12 12M18 6 6 18" />,
  send: <>
    <path d="M4 11.5 20 4l-6.5 16-2.4-6.1L4 11.5Z" />
    <path d="M11.1 13.9 20 4" />
  </>,
  sparkle: <>
    <path d="M12 3.2 13.7 8l4.8 1.7-4.8 1.7L12 16.2l-1.7-4.8L5.5 9.7 10.3 8 12 3.2Z" />
    <path d="M18.4 15.2l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8.8-2.2Z" />
  </>,
  coin: <>
    <ellipse cx="12" cy="6" rx="7.5" ry="3" />
    <path d="M4.5 6v6c0 1.7 3.4 3 7.5 3s7.5-1.3 7.5-3V6M4.5 12v6c0 1.7 3.4 3 7.5 3s7.5-1.3 7.5-3v-6" />
  </>,
  yuan: <>
    <path d="m7 3 5 7 5-7M12 10v11M6 12h12M6 16h12" />
  </>,
  efficiency: <>
    <path d="M4 19 19 4M5 4h5v5H5zM14 15h5v5h-5z" />
    <path d="M4 9V4h5M15 20h5v-5" />
  </>,
  factory: <>
    <path d="M3 20V9l6 3V8l6 4V4h6v16H3Z" />
    <path d="M7 16h2M12 16h2M17 16h2" />
  </>,
  cube: <>
    <path d="m12 2.8 8 4.5v9.4l-8 4.5-8-4.5V7.3l8-4.5Z" />
    <path d="m4.3 7.4 7.7 4.4 7.7-4.4M12 11.8v9.2" />
  </>,
  tag: <>
    <path d="M3.5 5.5v6.2l8.8 8.8 8.2-8.2-8.8-8.8H3.5Z" />
    <circle cx="7.4" cy="7.5" r="1.1" />
  </>,
  expand: <path d="M8 4H4v4M16 4h4v4M4 16v4h4M20 16v4h-4" />,
  user: <>
    <circle cx="12" cy="8" r="4" />
    <path d="M4.5 21c.5-5 3-7 7.5-7s7 2 7.5 7" />
  </>,
  layers: <>
    <path d="m12 3 9 5-9 5-9-5 9-5Z" />
    <path d="m3 12 9 5 9-5M3 16l9 5 9-5" />
  </>,
}

export default function Icon({ name, size = 18 }: { name: IconName; size?: number }) {
  return <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.6"
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden="true"
    focusable="false"
  >{paths[name]}</svg>
}
