<script lang="ts">
    import { pb } from "../lib/pocketbase";

    let file = $state<File | null>(null);
    let loading = $state(false);

    async function upload() {
        if (!file) return;
        loading = true;
        const data = new FormData();
        data.append("file", file);

        const res = await pb.send("/import_daily_transactions", {
            method: "POST",
            body: data
        })
        loading = false;
        console.log(res)
    }
</script>

<div class="container py-5">
    <div class="card border-0 shadow-sm mx-auto" style="max-width: 400px;">
        <div class="card-body p-4 text-center">
            <i class="bi bi-file-earmark-excel-fill text-success display-4 mb-3"></i>
            <h5 class="fw-bold">Import XLSX</h5>
            <input type="file" accept=".xlsx" class="form-control my-3" 
                onchange={(e) => file = (e.currentTarget as HTMLInputElement).files?.[0] || null} />
            
            <button onclick={upload} class="btn btn-primary w-100 fw-bold" disabled={!file || loading}>
                {#if loading} <span class="spinner-border spinner-border-sm me-2"></span> {/if}
                {loading ? 'Uploading...' : 'Upload Spreadsheet'}
            </button>
        </div>
    </div>
</div>