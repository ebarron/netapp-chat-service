// Recharts' ResponsiveContainer always initialises with width=-1/height=-1 during
// its first React render (before the DOM node exists) and emits a console.warn.
// This is a known recharts limitation — the chart renders correctly after mount.
// Filter out this specific warning to keep the console clean.
const _warn = console.warn;
console.warn = function rechartsFilter(...args: Parameters<typeof console.warn>) {
  if (typeof args[0] === 'string' && args[0].includes('of chart should be greater than 0')) return;
  _warn.apply(console, args);
};
