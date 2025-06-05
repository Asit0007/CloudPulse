// script.js - Fetches data and updates the CloudPulse dashboard

document.addEventListener('DOMContentLoaded', () => {
    console.log("CloudPulse Dashboard Initializing...");
    initializeDashboard();
    fetchAllData();

    // Optional: Set up a timer to refresh data periodically.
    // Be mindful of API call limits and data transfer for free tier.
    // A manual refresh button might be a better free-tier friendly option.
    // setInterval(fetchAllData, 300000); // Refresh every 5 minutes (300,000 ms)
});

function initializeDashboard() {
    // Set initial "Last Updated" for dynamic fields if not fetched yet
    document.getElementById('ec2-last-updated').textContent = 'Fetching...';
    document.getElementById('ft-last-updated').textContent = 'N/A (Manual Check)'; // Set once as it's not API driven
    document.getElementById('github-last-updated').textContent = 'Fetching...';
    
    // Update dynamic footer year
    const footer = document.querySelector('footer p');
    if (footer) {
        footer.textContent = `© ${new Date().getFullYear()} CloudPulse. All rights reserved. Powered by AWS Free Tier.`;
    }
}

/**
 * Updates the "Last Updated" timestamp for a given element.
 * @param {string} elementId The ID of the span element for the timestamp.
 */
function updateTimestamp(elementId) {
    const now = new Date();
    const timestampElement = document.getElementById(elementId);
    if (timestampElement) {
        timestampElement.textContent = now.toLocaleString();
    }
}

/**
 * Helper function to set text content of an element.
 * @param {string} elementId The ID of the element.
 * @param {string} text The text to set.
 * @param {string} [defaultValue='N/A'] The default text if data is null or undefined.
 */
function setText(elementId, text, defaultValue = 'N/A') {
    const element = document.getElementById(elementId);
    if (element) {
        element.textContent = (text === null || text === undefined || text === "") ? defaultValue : text;
    } else {
        console.warn(`Element with ID "${elementId}" not found.`);
    }
}

/**
 * Generic fetch function with error handling.
 * @param {string} url The API endpoint URL.
 * @returns {Promise<any>} A promise that resolves with the JSON data or rejects with an error.
 */
async function fetchData(url) {
    try {
        // Relative paths work when frontend is served by the Go backend.
        // For local file:// testing, you might need full URLs and CORS handling.
        const response = await fetch(url);
        if (!response.ok) {
            let errorMsg = `HTTP error! Status: ${response.status}`;
            try {
                const errorBody = await response.json(); // Try to parse error body as JSON
                errorMsg += `, Message: ${errorBody.error || JSON.stringify(errorBody)}`;
            } catch (e) {
                // If error body is not JSON or parsing fails
                const textBody = await response.text();
                errorMsg += `, Body: ${textBody}`;
            }
            throw new Error(errorMsg);
        }
        return await response.json();
    } catch (error) {
        console.error(`Failed to fetch from ${url}:`, error);
        throw error; // Re-throw to be caught by specific fetch functions
    }
}

/**
 * Fetches and displays EC2 usage data.
 */
async function fetchEC2Usage() {
    try {
        const data = await fetchData('/api/ec2-usage');
        console.log("EC2 Data Received:", data);

        setText('ec2-instance-id', data.InstanceID, 'N/A');
        setText('ec2-cpu', data.cpu !== 'N/A' && data.cpu !== undefined ? `${parseFloat(data.cpu).toFixed(2)}%` : 'N/A');
        setText('ec2-net-in', data.netIn !== 'N/A' && data.netIn !== undefined ? `${parseFloat(data.netIn).toLocaleString()} bytes` : 'N/A');
        setText('ec2-net-out', data.netOut !== 'N/A' && data.netOut !== undefined ? `${parseFloat(data.netOut).toLocaleString()} bytes` : 'N/A');
        
        // Memory and Disk are not provided by basic CloudWatch metrics via our current backend
        setText('ec2-memory', data.memUsed !== 'N/A' && data.memUsed !== undefined ? `${parseFloat(data.memUsed).toFixed(2)}%` : 'N/A');
        setText('ec2-disk', data.diskUsed !== 'N/A' && data.diskUsed !== undefined ? `${parseFloat(data.diskUsed).toFixed(2)}%` : 'N/A');
        
        if (data.message) { // Display any informational messages from backend
            const container = document.getElementById('ec2-data-container');
            let msgElement = container.querySelector('.api-message');
            if (!msgElement) {
                msgElement = document.createElement('p');
                msgElement.className = 'api-message loading-text'; // Reuse styling
                container.appendChild(msgElement);
            }
            msgElement.textContent = data.message;
        }


        updateTimestamp('ec2-last-updated');
    } catch (error) {
        console.error("Failed to display EC2 usage:", error);
        setText('ec2-instance-id', 'Error');
        setText('ec2-cpu', 'Error');
        setText('ec2-memory', 'Error');
        setText('ec2-disk', 'Error');
        setText('ec2-net-in', 'Error');
        setText('ec2-net-out', 'Error');
        document.getElementById('ec2-last-updated').textContent = 'Error';
    }
}

/**
 * Fetches and displays AWS Free Tier usage data.
 */
async function fetchFreeTierUsage() {
    try {
        const data = await fetchData('/api/free-tier-usage');
        setText('ft-ec2-hours-used', data.ec2HoursUsed !== undefined ? data.ec2HoursUsed : 'N/A');
        setText('ft-ec2-hours-remaining', data.ec2HoursRemaining !== undefined ? data.ec2HoursRemaining : 'N/A');
        setText('ft-data-transfer-out-used', data.dataTransferOutUsed !== undefined ? `${data.dataTransferOutUsed} GB` : 'N/A');
        setText('ft-data-transfer-out-remaining', data.dataTransferOutRemaining !== undefined ? `${data.dataTransferOutRemaining} GB` : 'N/A');
        updateTimestamp('ft-last-updated');
    } catch (error) {
        setText('ft-ec2-hours-used', 'Error');
        setText('ft-ec2-hours-remaining', 'Error');
        setText('ft-data-transfer-out-used', 'Error');
        setText('ft-data-transfer-out-remaining', 'Error');
        document.getElementById('ft-last-updated').textContent = 'Error';
        console.error("Failed to fetch Free Tier usage:", error);
    }
}


/**
 * Updates the Free Tier section (static info, no API call).
 */
function updateFreeTierInfo() {
    // These are placeholders as we don't fetch this data via API to stay free.
    // The HTML already contains guidance. This just updates "Loading..."
    setText('ft-ec2-hours-used', 'Check AWS Console');
    setText('ft-ec2-hours-remaining', 'Check AWS Console');
    setText('ft-data-transfer-out-used', 'Check AWS Console');
    setText('ft-data-transfer-out-remaining', 'Check AWS Console');
    // The 'ft-last-updated' is set once in initializeDashboard as it's not dynamic from API
}

/**
 * Fetches and displays GitHub repository users.
 */
async function fetchGitHubUsers() {
    const usersListElement = document.getElementById('github-users-list');
    try {
        const users = await fetchData('/api/github-users');
        console.log("GitHub Users Received:", users);

        // For GitHub repo name - backend would ideally supply this.
        // For now, using a placeholder or assuming it's from env if backend could pass it.
        // If your backend GITHUB_REPO env var is "cloudpulse", for example:
        setText('github-repo-name', 'Configured Repository'); // Or fetch dynamically if backend is updated

        if (!users || !Array.isArray(users)) {
            throw new Error("Invalid user data format from API.");
        }

        if (users.length === 0) {
            usersListElement.innerHTML = '<li>No users found or repository is private without sufficient token scope.</li>';
        } else {
            usersListElement.innerHTML = ''; // Clear "Loading..." or old list
            users.forEach(user => {
                const listItem = document.createElement('li');
                listItem.innerHTML = `
                    <img src="${user.avatar_url || 'placeholder.png'}" alt="${user.login || 'user'}" width="25" height="25" style="border-radius: 50%; vertical-align: middle; margin-right: 8px;">
                    <a href="${user.html_url || '#'}" target="_blank" rel="noopener noreferrer">${user.login || 'Unknown User'}</a>
                    (${user.role_name || 'Collaborator'})
                `;
                usersListElement.appendChild(listItem);
            });
        }
        updateTimestamp('github-last-updated');
    } catch (error) {
        console.error("Failed to display GitHub users:", error);
        usersListElement.innerHTML = `<li class="error-text">Error loading GitHub users: ${error.message}</li>`;
        setText('github-repo-name', 'Error');
        document.getElementById('github-last-updated').textContent = 'Error';
    }
}

/**
 * Main function to fetch all data for the dashboard.
 */
function fetchAllData() {
    console.log("Fetching all dashboard data...");
    fetchEC2Usage();
    fetchFreeTierUsage(); // Now fetches from API
    //updateFreeTierInfo(); // This is static, just updates placeholders
    fetchGitHubUsers();
}

// --- Notes specific to this updated script ---
// 1. EC2 Memory/Disk: These are set to "N/A (Agent Req.)" because CloudWatch Basic Monitoring
//    (which the backend uses for EC2 stats to stay free tier) does not provide these for Linux
//    without installing the CloudWatch Agent and configuring custom metrics (which can incur costs).
// 2. AWS Free Tier Usage: This section is explicitly informational. The `updateFreeTierInfo` function
//    clarifies that users should check the AWS Console, reinforcing the HTML messages.
//    No API calls are made for this to ensure zero AWS cost.
// 3. GitHub Repo Name: The `github-repo-name` span is set to a placeholder. To make this dynamic,
//    your `/api/github-users` backend endpoint would need to be modified to include the repository
//    name (which it can get from its GITHUB_REPO environment variable) in its response,
//    or a new dedicated API endpoint could provide this.
// 4. Error Handling: Basic error handling is included to update fields to "Error" if fetches fail.
//    More sophisticated UI error messages could be implemented in the `showError` style if preferred.
