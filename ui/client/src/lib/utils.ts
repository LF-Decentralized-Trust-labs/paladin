import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
dayjs.extend(relativeTime);

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/**
 * Convert text to truncated length
 * @param hash text to truncate
 * @param truncateLength length to truncate. If 0, no truncation occurs
 * @returns truncated text
 */
export const makeHashText = (hash: string, truncate = true) => {
  if (hash.length <= 8 || !truncate) {
    return hash;
  }
  return `${hash.slice(0, 5)}...${hash.slice(-3)}`;
};

/**
 * Get time from unix timestamps
 * @param ts timestamp
 * @param fullLength if full timestamp should be returned
 * @returns timestamp
 */
export const getPaladinTime = (ts: string, fullLength?: boolean): string => {
  return fullLength
    ? dayjs(ts).format("MM/DD/YYYY h:mm:ss.SSS A")
    : dayjs(ts).fromNow();
};
