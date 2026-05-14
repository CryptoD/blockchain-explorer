/**
 * Tree-shaken Chart.js for portfolio dashboard (line, doughnut, bar).
 * Loaded via dynamic import from /static/js/dashboard.js.
 */
import {
  Chart,
  LineController,
  LineElement,
  PointElement,
  LinearScale,
  CategoryScale,
  BarController,
  BarElement,
  DoughnutController,
  ArcElement,
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
  BarController,
  BarElement,
  DoughnutController,
  ArcElement,
  Tooltip,
  Legend,
  Filler
);

export { Chart };
