// Dashboard client behaviour: dark-mode toggle (persisted) and expandable
// transaction descriptions. The initial theme is applied by an inline script in
// <head> to avoid a flash; this file wires up the toggle button and rows.
(function () {
	function currentTheme() {
		return document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light';
	}

	function updateToggle() {
		var btn = document.getElementById('theme-toggle');
		if (btn) {
			btn.textContent = currentTheme() === 'dark' ? '☀️' : '🌙';
		}
	}

	function toggleTheme() {
		var next = currentTheme() === 'dark' ? 'light' : 'dark';
		document.documentElement.setAttribute('data-theme', next);
		try { localStorage.setItem('theme', next); } catch (e) {}
		updateToggle();
	}

	// Show the full (often truncated) description as a native tooltip and let a
	// click expand the cell inline.
	function enhanceDescriptions(root) {
		var cells = (root || document).querySelectorAll('td.desc-cell');
		for (var i = 0; i < cells.length; i++) {
			var td = cells[i];
			if (!td.title) {
				td.title = td.textContent.trim();
			}
			if (!td.dataset.enhanced) {
				td.dataset.enhanced = '1';
				td.addEventListener('click', function () {
					this.classList.toggle('expanded');
				});
			}
		}
	}

	document.addEventListener('DOMContentLoaded', function () {
		updateToggle();
		enhanceDescriptions();
		var btn = document.getElementById('theme-toggle');
		if (btn) {
			btn.addEventListener('click', toggleTheme);
		}
	});

	// htmx swaps in new transaction rows; re-run the enhancement on them.
	document.addEventListener('htmx:afterSwap', function (e) {
		enhanceDescriptions(e.target);
	});
})();
