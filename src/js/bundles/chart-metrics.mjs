/**
 * Tree-shaken Chart.js for blockchain metrics (line charts only).
 * Loaded via dynamic import from /static/js/script.js on the explorer page.
 */
import {
  Chart,
  LineController,
  LineElement,
  PointElement,
  LinearScale,
  CategoryScale,
  Tooltip,
  Legend,
  Filler,
} from "chart.js";

Chart.register(
  LineController,
  LineElement,
  PointElement,
  LinearScale,
  CategoryScale,
  Tooltip,
  Legend,
  Filler
);

export { Chart };
