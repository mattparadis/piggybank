// Dashboard client behaviour: dark-mode toggle (persisted), expandable
// transaction descriptions, toast feedback for htmx requests and the
// rule-form open/close toggle. The initial theme is applied by an inline
// script in <head> to avoid a flash.
(function () {
	var toastTimer;
	function showToast(msg, kind) {
		var el = document.getElementById('toast');
		if (!el) return;
		el.textContent = msg;
		el.className = 'toast ' + (kind || 'ok');
		el.hidden = false;
		clearTimeout(toastTimer);
		toastTimer = setTimeout(function () { el.hidden = true; }, 3500);
	}

	document.addEventListener('htmx:responseError', function (e) {
		var body = e.detail.xhr && e.detail.xhr.responseText;
		showToast((body || 'Request failed').slice(0, 200), 'err');
	});
	document.addEventListener('htmx:sendError', function () {
		showToast('Network error', 'err');
	});
	// Fired by htmx from the HX-Trigger response header.
	document.addEventListener('toast', function (e) {
		var d = e.detail || {};
		showToast(d.message || 'Done', d.kind || 'ok');
	});

	// hx-trigger filter for the ≡ button: close an already-open rule form
	// instead of fetching a second one (global: referenced from the attribute).
	window.toggleRuleSlot = function (el) {
		var cell = el.closest('.cat-cell');
		var slot = cell && cell.querySelector('.rule-slot');
		if (slot && slot.firstElementChild) {
			slot.innerHTML = '';
			return false;
		}
		return true;
	};
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
	// When the rule form is swapped in, focus its keyword input.
	document.addEventListener('htmx:afterSwap', function (e) {
		enhanceDescriptions(e.target);
		if (e.target.classList && e.target.classList.contains('rule-slot')) {
			var kw = e.target.querySelector('input[name=keyword]');
			if (kw) kw.focus();
		}
	});
})();
