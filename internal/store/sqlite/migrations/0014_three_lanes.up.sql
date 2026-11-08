UPDATE tickets SET lane = 'in_progress' WHERE lane = 'review' OR stage = 'pr';
