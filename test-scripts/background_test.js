#!/usr/bin/env node
/**
 * Background Process Test Script (Node.js)
 *
 * This script runs for 5 minutes and logs a random string every second.
 * Works universally across all operating systems with Node.js installed.
 */

const crypto = require('crypto');

function generateRandomString(length = 10) {
    return crypto.randomBytes(length).toString('hex').substring(0, length);
}

function main() {
    console.log('🚀 Background test process started!');
    console.log(`⏰ Start time: ${new Date().toISOString()}`);
    console.log('📝 Logging random strings every second for 5 minutes...');
    console.log('-'.repeat(50));

    const startTime = Date.now();
    const duration = 5 * 60 * 1000; // 5 minutes in milliseconds
    let counter = 0;

    const interval = setInterval(() => {
        counter++;
        const currentTime = new Date().toISOString();
        const randomString = generateRandomString(12);

        console.log(`[${counter.toString().padStart(4, '0')}] ${currentTime} | Random: ${randomString}`);

        // Check if we've reached the duration
        if (Date.now() - startTime >= duration) {
            clearInterval(interval);
            finish();
        }
    }, 1000);

    // Handle process interruption
    process.on('SIGINT', () => {
        clearInterval(interval);
        console.log('\n🛑 Process interrupted by user');
        finish();
    });

    process.on('SIGTERM', () => {
        clearInterval(interval);
        console.log('\n🛑 Process terminated');
        finish();
    });

    function finish() {
        const elapsed = (Date.now() - startTime) / 1000;
        console.log(`\n🏁 Process finished after ${elapsed.toFixed(2)} seconds`);
        console.log(`📊 Total iterations: ${counter}`);
        console.log(`⏰ End time: ${new Date().toISOString()}`);
        process.exit(0);
    }
}

main();
