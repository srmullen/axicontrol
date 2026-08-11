// Drag-and-drop upload support for #upload-dropzone. Listens on document
// rather than the dropzone element directly because htmx swaps that element
// (outerHTML) on every upload, which would otherwise drop directly-attached
// listeners.
(function () {
	function findDropzone(target) {
		return target && target.closest ? target.closest("#upload-dropzone") : null;
	}

	document.addEventListener("dragover", function (evt) {
		if (!findDropzone(evt.target)) return;
		evt.preventDefault();
	});

	document.addEventListener("drop", function (evt) {
		var dropzone = findDropzone(evt.target);
		if (!dropzone) return;
		evt.preventDefault();

		var errorEl = dropzone.querySelector("#upload-dropzone-error");
		var files = evt.dataTransfer ? evt.dataTransfer.files : null;
		if (!files || files.length === 0) return;

		if (files.length > 1) {
			if (errorEl) errorEl.textContent = "Drop a single SVG file at a time.";
			return;
		}

		if (errorEl) errorEl.textContent = "";

		var input = dropzone.querySelector('input[type="file"]');
		var form = dropzone.querySelector("form");
		if (!input || !form) return;

		var transfer = new DataTransfer();
		transfer.items.add(files[0]);
		input.files = transfer.files;

		form.requestSubmit();
	});
})();
