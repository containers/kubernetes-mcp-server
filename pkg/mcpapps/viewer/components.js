(function() {
  'use strict';

  var hp = window.htmPreact;
  var html = hp.html;
  var useState = hp.useState;
  var useEffect = hp.useEffect;
  var useMemo = hp.useMemo;
  var useCallback = hp.useCallback;
  var useRef = hp.useRef;

  // SortableTable: renders array-of-objects as a sortable HTML table
  function SortableTable(props) {
    var data = props.data;
    var columns = props.columns;
    var sortState = useState({ col: null, asc: true });
    var sortCol = sortState[0].col;
    var sortAsc = sortState[0].asc;
    var setSort = sortState[1];

    var sorted = useMemo(function() {
      if (!sortCol || !data) return data;
      return data.slice().sort(function(a, b) {
        var av = a[sortCol], bv = b[sortCol];
        if (av == null) return 1;
        if (bv == null) return -1;
        if (typeof av === 'number' && typeof bv === 'number') return sortAsc ? av - bv : bv - av;
        var sa = String(av), sb = String(bv);
        return sortAsc ? sa.localeCompare(sb) : sb.localeCompare(sa);
      });
    }, [data, sortCol, sortAsc]);

    var handleSort = useCallback(function(col) {
      setSort(function(prev) {
        if (prev.col === col) return { col: col, asc: !prev.asc };
        return { col: col, asc: true };
      });
    }, []);

    if (!data || data.length === 0) {
      return html`<p class="status">No data</p>`;
    }

    return html`
      <div class="count">${data.length} item${data.length !== 1 ? 's' : ''}</div>
      <table>
        <thead>
          <tr>
            ${columns.map(function(c) {
              var arrow = sortCol === c.key ? (sortAsc ? '\u25B2' : '\u25BC') : '';
              return html`<th key=${c.key} onClick=${function() { handleSort(c.key); }}>
                ${c.label}<span class="sort-arrow">${arrow}</span>
              </th>`;
            })}
          </tr>
        </thead>
        <tbody>
          ${sorted.map(function(row, i) {
            return html`<tr key=${i}>
              ${columns.map(function(c) {
                return html`<td key=${c.key}>${row[c.key] != null ? String(row[c.key]) : ''}</td>`;
              })}
            </tr>`;
          })}
        </tbody>
      </table>
    `;
  }

  // TableView: generic table for any structured array data
  // Automatically derives columns from the keys of the first data item.
  function TableView(props) {
    var data = props.data;
    // useMemo must be called unconditionally (rules of hooks)
    var columns = useMemo(function() {
      if (!Array.isArray(data) || data.length === 0) return [];
      var keys = Object.keys(data[0]);
      return keys.map(function(k) {
        return { key: k, label: k };
      });
    }, [data]);
    if (!Array.isArray(data) || data.length === 0) {
      return html`<p class="status">No data found</p>`;
    }
    return html`<${SortableTable} data=${data} columns=${columns} />`;
  }

  // Parse a metric value string into a numeric value based on its unit type.
  function parseUnit(value, unit) {
    var raw = String(value || '0');
    if (unit === 'cpu') {
      if (raw.endsWith('m')) return parseFloat(raw);
      if (raw.endsWith('n')) return parseFloat(raw) / 1e6;
      return parseFloat(raw) * 1000;
    }
    if (unit === 'memory') {
      if (raw.endsWith('Mi')) return parseFloat(raw);
      if (raw.endsWith('Ki')) return parseFloat(raw) / 1024;
      if (raw.endsWith('Gi')) return parseFloat(raw) * 1024;
      return parseFloat(raw) / (1024 * 1024);
    }
    return parseFloat(raw) || 0;
  }

  // Dataset colors used for chart rendering (cycled when more than 2 datasets)
  var datasetColors = [
    { bg: 'rgba(59, 130, 246, 0.7)', border: 'rgba(59, 130, 246, 1)' },
    { bg: 'rgba(16, 185, 129, 0.7)', border: 'rgba(16, 185, 129, 1)' }
  ];

  // MetricsTable: data-driven metrics chart + table from self-describing structured content
  function MetricsTable(props) {
    var data = props.data;
    var items = data.items;
    var columns = data.columns;
    var chartCfg = data.chart;
    var canvasRef = useRef(null);
    var chartRef = useRef(null);

    useEffect(function() {
      if (!items || items.length === 0 || !canvasRef.current || typeof Chart === 'undefined' || !chartCfg) return;

      // Destroy previous chart instance
      if (chartRef.current) {
        chartRef.current.destroy();
        chartRef.current = null;
      }

      var labelKey = chartCfg.labelKey || 'name';
      var labels = items.map(function(d) { return d[labelKey] || ''; });
      var datasets = chartCfg.datasets.map(function(ds, i) {
        var color = datasetColors[i % datasetColors.length];
        return {
          label: ds.label,
          data: items.map(function(d) { return parseUnit(d[ds.key], ds.unit); }),
          backgroundColor: color.bg,
          borderColor: color.border,
          borderWidth: 1,
          yAxisID: ds.axis === 'right' ? 'y1' : 'y'
        };
      });

      // Detect dark mode from the document's color-scheme (set by applyTheme in app.js)
      var isDark = document.documentElement.style.colorScheme === 'dark' ||
        document.documentElement.getAttribute('data-theme') === 'dark';
      var textColor = isDark ? '#e5e7eb' : '#374151';
      var gridColor = isDark ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.1)';

      // Build scales from dataset axis/label descriptors
      var scales = {
        x: { ticks: { color: textColor }, grid: { color: gridColor } }
      };
      var hasLeft = false, hasRight = false;
      chartCfg.datasets.forEach(function(ds) {
        if (ds.axis === 'right' && !hasRight) {
          hasRight = true;
          scales.y1 = {
            type: 'linear', position: 'right',
            title: { display: true, text: ds.label, color: textColor },
            beginAtZero: true, ticks: { color: textColor },
            grid: { drawOnChartArea: false }
          };
        } else if (!hasLeft) {
          hasLeft = true;
          scales.y = {
            type: 'linear', position: 'left',
            title: { display: true, text: ds.label, color: textColor },
            beginAtZero: true, ticks: { color: textColor },
            grid: { color: gridColor }
          };
        }
      });

      chartRef.current = new Chart(canvasRef.current, {
        type: 'bar',
        data: { labels: labels, datasets: datasets },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          color: textColor,
          plugins: {
            legend: { position: 'top', labels: { color: textColor } }
          },
          scales: scales
        }
      });

      return function() {
        if (chartRef.current) {
          chartRef.current.destroy();
          chartRef.current = null;
        }
      };
    }, [data]);

    if (!items || items.length === 0) {
      return html`<p class="status">No metrics data</p>`;
    }
    return html`
      <div class="chart-container">
        <canvas ref=${canvasRef} height="250"></canvas>
      </div>
      <${SortableTable} data=${items} columns=${columns} />
    `;
  }

  // GenericView: fallback for unknown tools, shows raw text
  function GenericView(props) {
    return html`<pre class="raw">${props.text || 'No content'}</pre>`;
  }

  // YamlView: syntax-highlighted YAML using Prism.js
  function YamlView(props) {
    var highlighted = useMemo(function() {
      if (!props.text || typeof Prism === 'undefined' || !Prism.languages.yaml) return null;
      return Prism.highlight(props.text, Prism.languages.yaml, 'yaml');
    }, [props.text]);
    if (!highlighted) {
      return html`<${GenericView} text=${props.text} />`;
    }
    return html`<pre class="raw yaml"
      dangerouslySetInnerHTML=${{ __html: highlighted }} />`;
  }

  // Expose components
  window.mcpComponents = {
    SortableTable: SortableTable,
    TableView: TableView,
    MetricsTable: MetricsTable,
    GenericView: GenericView,
    YamlView: YamlView
  };
})();
